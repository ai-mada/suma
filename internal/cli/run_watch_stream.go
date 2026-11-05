package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	runWatchFrameLimit          = 16 * 1024 * 1024
	runWatchSequenceLimit int64 = 9007199254740991
)

func (c *client) readRunEventStream(ctx context.Context, run string, after int64, emit func(json.RawMessage, int64) error) (bool, error) {
	q := url.Values{"after": {strconv.FormatInt(after, 10)}}
	req, err := c.buildRequest(ctx, http.MethodGet, "/api/v1/runs/"+url.PathEscape(run)+"/events/stream?"+q.Encode(), nil, true)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.http.Do(req.WithContext(ctx))
	if err != nil {
		return false, controlTransportFailure(ctx, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, err := readControlResponseBody(resp)
		if err != nil {
			return false, err
		}
		return false, controlHTTPFailure(resp.StatusCode, raw)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || err != nil || mediaType != "text/event-stream" ||
		params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		return false, runWatchProtocolFailure("live run event response must be a UTF-8 event stream")
	}
	return readRunWatchFrames(ctx, resp.Body, run, after, emit)
}

type runWatchFrame struct {
	kind, id           string
	data               []byte
	hasData, heartbeat bool
}

// Only blank-line-terminated frames are emitted. Count the received bytes,
// including framing and comments, before buffering a long line.
func readRunWatchFrames(ctx context.Context, body io.Reader, run string, after int64, emit func(json.RawMessage, int64) error) (activity bool, err error) {
	reader := bufio.NewReader(body)
	var frame runWatchFrame
	var partialLine []byte
	frameBytes, firstLine := 0, true
	for {
		line, err := reader.ReadSlice('\n')
		frameBytes += len(line)
		if frameBytes > runWatchFrameLimit {
			return activity, runWatchProtocolFailure("live run event frame exceeds 16 MiB")
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			partialLine = append(partialLine, line...)
			continue
		}
		if errors.Is(err, io.EOF) {
			return activity, nil
		}
		if err != nil {
			return activity, controlTransportFailure(ctx, false)
		}
		if partialLine != nil {
			line = append(partialLine, line...)
			partialLine = nil
		}
		if firstLine {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			firstLine = false
		}
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'})
		if !utf8.Valid(line) {
			return activity, runWatchProtocolFailure("invalid live run event frame")
		}
		if len(line) != 0 {
			frame.consumeLine(line)
			continue
		}
		if frame.hasData || frame.id != "" || frame.kind == "run_event" || frame.kind == "stream_error" {
			sequence, err := frame.sequence(run)
			if err != nil {
				return activity, err
			}
			if err := emit(json.RawMessage(frame.data), sequence); err != nil {
				return activity, err
			}
			if sequence > after {
				activity, after = true, sequence
			}
		} else if frame.heartbeat {
			activity = true
		}
		frame, frameBytes = runWatchFrame{}, 0
	}
}

func (f *runWatchFrame) consumeLine(line []byte) {
	field, value, _ := bytes.Cut(line, []byte{':'})
	value = bytes.TrimPrefix(value, []byte{' '})
	switch string(field) {
	case "":
		// Comments are transport facts. The initial connected comment does
		// not prove the stream delivered a heartbeat or made progress.
		f.heartbeat = f.heartbeat || bytes.Equal(value, []byte("heartbeat"))
	case "event":
		f.kind = string(value)
	case "id":
		f.id = string(value)
	case "data":
		if f.hasData {
			f.data = append(f.data, '\n')
		}
		f.data = append(f.data, value...)
		f.hasData = true
	}
}

func (f runWatchFrame) sequence(run string) (int64, error) {
	const invalid = "invalid live run event frame"
	switch f.kind {
	case "stream_error":
		var e struct {
			Status int    `json:"status"`
			Code   string `json:"code"`
		}
		if f.id != "" || json.Unmarshal(f.data, &e) != nil || e.Status < 400 || e.Status > 599 || e.Code == "" || len(e.Code) > 128 {
			return 0, runWatchProtocolFailure(invalid)
		}
		return 0, &failure{Code: e.Code, Status: e.Status, Message: "live run event stream reported an error"}
	case "run_event":
		sequence, err := strconv.ParseInt(f.id, 10, 64)
		if err != nil || sequence < 1 || sequence > runWatchSequenceLimit || strconv.FormatInt(sequence, 10) != f.id {
			return 0, runWatchProtocolFailure(invalid)
		}
		var event struct {
			Sequence int64           `json:"sequence"`
			RunID    string          `json:"run_id"`
			NodeID   *string         `json:"node_id"`
			Type     string          `json:"type"`
			At       string          `json:"at"`
			Data     json.RawMessage `json:"data"`
		}
		if json.Unmarshal(f.data, &event) != nil || event.Sequence != sequence || event.RunID != run ||
			event.NodeID == nil || event.Type == "" || event.At == "" || !bytes.HasPrefix(bytes.TrimSpace(event.Data), []byte{'{'}) {
			return 0, runWatchProtocolFailure(invalid)
		}
		return sequence, nil
	default:
		// A complete unknown frame with data or an ID may carry a persisted
		// event. Never skip it and then acknowledge a later cursor.
		return 0, runWatchProtocolFailure(invalid)
	}
}

func runWatchProtocolFailure(message string) *failure {
	return &failure{Code: "run_watch_protocol", Message: message}
}
