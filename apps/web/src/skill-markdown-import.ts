import {isMap, parseDocument, visit} from 'yaml';
import {UiError} from './i18n';

const maxFileBytes = 32 * 1024;
const frontmatterBoundary = /^---[ \t]*$/;
const skillName = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/** Read one local Agent Skills entry point into the existing knowledge draft. */
export async function readSkillMarkdown(file: File): Promise<{title: string; summary: string; content: string}> {
    if (file.name.length !== 8 || !/^skill\.md$/i.test(file.name)) {
        throw new UiError('knowledge.import_invalid');
    }
    if (file.size > maxFileBytes) throw new UiError('knowledge.import_too_large');

    let bytes: ArrayBuffer;
    try {
        bytes = await file.arrayBuffer();
    } catch {
        throw new UiError('knowledge.import_read_failed');
    }
    if (bytes.byteLength > maxFileBytes) throw new UiError('knowledge.import_too_large');

    try {
        const text = new TextDecoder('utf-8', {fatal: true}).decode(bytes);
        return parseSkillMarkdown(text);
    } catch (error) {
        if (error instanceof UiError) throw error;
        throw new UiError('knowledge.import_invalid');
    }
}

function parseSkillMarkdown(text: string) {
    const lines = text.replace(/\r\n/g, '\n').split('\n');
    const end = lines.findIndex((line, index) => index > 0 && frontmatterBoundary.test(line));
    if (text.includes('\0') || !frontmatterBoundary.test(lines[0]) || end < 2) {
        throw new UiError('knowledge.import_invalid');
    }

    const header = parseDocument(lines.slice(1, end).join('\n'), {version: '1.2', uniqueKeys: true});
    if (header.errors.length || header.warnings.length || !isMap(header.contents)) {
        throw new UiError('knowledge.import_invalid');
    }
    // Read only the two supported scalar values and reject aliases anywhere in
    // the header without constructing or expanding the optional metadata.
    visit(header, {Alias: () => { throw new UiError('knowledge.import_invalid'); }});
    const title = header.get('name');
    const summary = header.get('description');
    const content = lines.slice(end + 1).join('\n');
    if (typeof title !== 'string' || title.length > 64 || title !== title.trim() || !skillName.test(title) ||
        typeof summary !== 'string' || !summary.trim() || /[\0\uD800-\uDFFF]/u.test(summary) || !content.trim()) {
        throw new UiError('knowledge.import_invalid');
    }

    // Keep the parsed fields within the server's UTF-8 knowledge draft limits.
    const encoder = new TextEncoder();
    if (encoder.encode(summary).length > 1000 || encoder.encode(content).length > 16000) {
        throw new UiError('knowledge.import_too_large');
    }
    return {title, summary, content};
}
