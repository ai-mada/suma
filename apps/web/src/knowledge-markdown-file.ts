import {UiError} from './i18n';

const maxFileBytes = 32 * 1024;
const formats = {
    skill: {
        name: /^skill\.md$/i,
        invalid: 'knowledge.import_invalid',
        tooLarge: 'knowledge.import_too_large',
        readFailed: 'knowledge.import_read_failed',
    },
    helixSpec: {
        name: /^spec\.md$/i,
        invalid: 'knowledge.helix_spec_invalid',
        tooLarge: 'knowledge.helix_spec_too_large',
        readFailed: 'knowledge.helix_spec_read_failed',
    },
};

export async function readKnowledgeMarkdownFile(file: File, format: keyof typeof formats): Promise<string> {
    const {name, invalid, tooLarge, readFailed} = formats[format];
    if (name.exec(file.name)?.[0] !== file.name) throw new UiError(invalid);
    if (file.size > maxFileBytes) throw new UiError(tooLarge);

    let bytes: ArrayBuffer;
    try {
        bytes = await file.arrayBuffer();
    } catch {
        throw new UiError(readFailed);
    }
    if (bytes.byteLength > maxFileBytes) throw new UiError(tooLarge);

    try {
        return new TextDecoder('utf-8', {fatal: true}).decode(bytes);
    } catch {
        throw new UiError(invalid);
    }
}
