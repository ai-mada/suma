package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/ai-mada/suma/internal/localization"
)

var skillCatalogSHA = regexp.MustCompile(`^[a-f0-9]{64}$`)

func skillCatalogCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help") {
		_, err := io.WriteString(out, skillCatalogHelp(c.locale))
		return err
	}
	if len(args) == 0 {
		return usage("skill-catalog requires list, get, revisions, promote, update, retire, or adopt; use skill-catalog --help")
	}
	const base = "/api/v1/skill-catalog"
	if args[0] == "list" {
		fs := commandFlags("skill-catalog list")
		domain := fs.String("domain", "", "domain filter")
		scenario := fs.String("scenario", "", "scenario filter")
		tag := fs.String("tag", "", "tag filter")
		state := fs.String("state", "", "published or retired")
		query := fs.String("query", "", "keyword query")
		limit := fs.Int("limit", 20, "page size: 1..100")
		cursor := fs.String("cursor", "", "opaque next_cursor")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *limit < 1 || *limit > 100 || *state != "" && *state != "published" && *state != "retired" {
			return usage("invalid skill-catalog read options; limit must be 1..100 and state published or retired")
		}
		q := url.Values{"limit": {strconv.Itoa(*limit)}}
		for name, value := range map[string]string{"domain": *domain, "scenario": *scenario, "tag": *tag, "state": *state, "q": *query, "cursor": *cursor} {
			if value != "" {
				q.Set(name, value)
			}
		}
		return contextRead(ctx, c, base+"?"+q.Encode(), out)
	}
	if args[0] == "get" || args[0] == "revisions" {
		if len(args) < 2 || !handoffCLIIdentifier(args[1], false) {
			return usage("skill-catalog requires a valid catalog ID")
		}
		path := base + "/" + url.PathEscape(args[1])
		if args[0] == "revisions" {
			return contextHistory(ctx, c, path+"/revisions", args[2:], out)
		}
		if len(args) != 2 {
			return usage("skill-catalog get accepts only the catalog ID")
		}
		return contextRead(ctx, c, path, out)
	}
	method, path, offset := http.MethodPost, base, 1
	switch args[0] {
	case "promote":
	case "update", "retire", "adopt":
		if len(args) < 2 || !handoffCLIIdentifier(args[1], false) {
			return usage("skill-catalog requires a valid catalog ID")
		}
		path += "/" + url.PathEscape(args[1])
		offset = 2
		if args[0] == "update" {
			method = http.MethodPatch
		} else if args[0] == "retire" {
			path += "/commands"
		} else {
			path += "/adoptions"
		}
	default:
		return usage("skill-catalog requires list, get, revisions, promote, update, retire, or adopt; use skill-catalog --help")
	}
	fs := commandFlags("skill-catalog " + args[0])
	file := fs.String("file", "", "UTF-8 JSON file, or - for bounded stdin")
	confirm := fs.Bool("confirm", false, "confirm this explicit governed write")
	if fs.Parse(args[offset:]) != nil || fs.NArg() != 0 || *file == "" {
		return usage("skill-catalog writes require --file JSON_FILE or --file - and --confirm")
	}
	if !*confirm {
		return usage("skill-catalog writes require explicit --confirm; approval and draft publication remain server-controlled")
	}
	body, err := readObject(*file, in)
	if err != nil {
		return err
	}
	if err = skillCatalogWriteBasis(args[0], body); err != nil {
		return err
	}
	if err = c.operation(method, path, body); err != nil {
		return err
	}
	raw, err := c.request(ctx, method, path, body, true)
	if err != nil {
		return err
	}
	return writeJSON(out, raw, c.token)
}

func skillCatalogInteger(body map[string]any, name string, minimum int64) bool {
	value, ok := body[name].(json.Number)
	if !ok {
		return false
	}
	n, err := value.Int64()
	return err == nil && n >= minimum
}

func skillCatalogWriteBasis(action string, body map[string]any) error {
	invalid := func() error {
		return usage("skill-catalog writes require explicit integer versions, fixed SHA256, reason, and an 8..128 character idempotency_key")
	}
	reason, reasonOK := body["reason"].(string)
	operation, keyOK := body["idempotency_key"].(string)
	if !reasonOK || !handoffCLIText(reason) || !keyOK || !handoffCLIOperation(operation) {
		return invalid()
	}
	minimum := int64(1)
	if action == "promote" {
		minimum = 0
	}
	if !skillCatalogInteger(body, "expected_version", minimum) {
		return invalid()
	}
	if action == "promote" {
		if value, _ := body["expected_version"].(json.Number); value.String() != "0" {
			return invalid()
		}
	}
	switch action {
	case "promote", "update":
		id, idOK := body["knowledge_id"].(string)
		sha, shaOK := body["published_sha256"].(string)
		if !idOK || !handoffCLIIdentifier(id, false) || !shaOK || !skillCatalogSHA.MatchString(sha) || !skillCatalogInteger(body, "expected_knowledge_version", 1) || !skillCatalogInteger(body, "published_revision", 1) {
			return invalid()
		}
		if acknowledged, ok := body["acknowledge_sharing"].(bool); !ok || !acknowledged {
			return usage("skill-catalog promote and update require acknowledge_sharing=true in JSON")
		}
	case "retire":
		if value, exists := body["action"]; exists && value != "retire" {
			return invalid()
		}
		body["action"] = "retire"
	case "adopt":
		if sha, ok := body["origin_sha256"].(string); !ok || !skillCatalogSHA.MatchString(sha) {
			return invalid()
		}
		if acknowledged, ok := body["acknowledge_draft"].(bool); !ok || !acknowledged {
			return usage("skill-catalog adopt requires acknowledge_draft=true in JSON")
		}
	}
	return nil
}

func skillCatalogHelp(locale localization.Locale) string {
	if locale == localization.EnUS {
		return skillCatalogHelpEN
	}
	return skillCatalogHelpZH
}

const skillCatalogHelpEN = `
Organization skill catalog (global --project and original-view flags go first):
  skill-catalog list [--domain KEY] [--scenario KEY] [--tag KEY]
                     [--state published|retired] [--query TEXT] [--limit 1..100] [--cursor NEXT_CURSOR]
  skill-catalog get ID
  skill-catalog revisions ID [--limit 1..100] [--cursor NEXT_CURSOR]
  skill-catalog promote --file request.json --confirm
  skill-catalog update|retire|adopt ID --file request.json --confirm

All reads require a currently accessible project. Promotion/update use the source project;
adoption uses the target project and creates an unpublished project-knowledge draft.
JSON must carry explicit expected_version, reason and an 8..128 character idempotency_key.
Promotion uses expected_version=0; update/retire/adopt require a positive catalog version.
Promotion/update also fix knowledge_id, expected_knowledge_version, published_revision,
published_sha256, domain, scenarios, tags and acknowledge_sharing=true.
Adoption fixes origin_sha256 and acknowledge_draft=true. Retire sends action=retire.
--file - reads bounded UTF-8 JSON from stdin. Version numbers, SHA and caller keys are retained.
Browser approval/current access are enforced by the server; --confirm grants no authority.
No preview POST, automatic publication, account refresh, script execution or installation occurs.
Unknown writes require the same caller, path, exact body and operation key; no automatic retry.
`

const skillCatalogHelpZH = `
组织技能目录（--project 与原页面身份等全局参数放在命令前）：
  skill-catalog list [--domain KEY] [--scenario KEY] [--tag KEY]
                     [--state published|retired] [--query TEXT] [--limit 1..100] [--cursor NEXT_CURSOR]
  skill-catalog get ID
  skill-catalog revisions ID [--limit 1..100] [--cursor NEXT_CURSOR]
  skill-catalog promote --file request.json --confirm
  skill-catalog update|retire|adopt ID --file request.json --confirm

读取也需要当前有访问权的项目。共享／更新使用源项目；采用使用目标项目，
只创建尚未发布的项目知识草稿，随后仍需目标项目正常审核发布。
JSON 须显式提供 expected_version、reason 和 8–128 字符 idempotency_key。
首次共享 expected_version=0；更新／退役／采用须提供目录正整数版本。
共享／更新还须固定 knowledge_id、expected_knowledge_version、published_revision、
published_sha256、domain、scenarios、tags，并明确 acknowledge_sharing=true。
采用须固定 origin_sha256 并明确 acknowledge_draft=true；退役发送 action=retire。
--file - 从标准输入读取有界 UTF-8 JSON，版本数字、摘要和原操作标识保持。
服务端重核浏览器批准与当前权限，--confirm 不授予权限。
不会发送预览 POST、自动发布、刷新身份、执行或安装脚本。
结果未知时保留原调用者、路径、相同正文和操作标识，不自动重试。
`
