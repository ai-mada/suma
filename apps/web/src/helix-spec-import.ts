import {UiError} from './i18n';
import {readKnowledgeMarkdownFile} from './knowledge-markdown-file';

/** Preview a HelixHarness specification as a manual project knowledge draft. */
export async function readHelixSpec(file: File): Promise<{title: string; summary: string; content: string}> {
    const content = (await readKnowledgeMarkdownFile(file, 'helixSpec')).replace(/\r\n/g, '\n');
    if (!/\P{White_Space}/u.test(content) || content.includes('\0')) {
        throw new UiError('knowledge.helix_spec_invalid');
    }

    const encoder = new TextEncoder();
    if (encoder.encode(content).length > 16000) throw new UiError('knowledge.helix_spec_too_large');

    const firstLine = content.split('\n').find(line => /\P{White_Space}/u.test(line));
    const heading = firstLine && /^#[ \t]/.test(firstLine)
        ? firstLine.slice(2).replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '')
        : '';
    const title = heading && encoder.encode(heading).length <= 200 ? heading : 'HelixHarness spec';
    return {title, summary: '', content};
}
