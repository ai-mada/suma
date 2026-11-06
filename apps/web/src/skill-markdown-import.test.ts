import {expect, it, vi} from 'vitest';
import {readSkillMarkdown} from './skill-markdown-import';

function skill(summary = 'Review code.', body = '# Instructions\n') {
    return new File(['---\nname: candidate-review\ndescription: ' + summary + '\n---\n' + body], 'SKILL.md');
}

it('reads BOM and CRLF, preserves Markdown lines, and ignores optional metadata', async () => {
    const body = '\n# 审查步骤\n\n    indented  \n---\nEnd\n';
    const text = '\uFEFF---\nname: candidate-review\ndescription: >-\n  Inspect the candidate\n  before review.\n' +
        'allowed-tools: shell\nmetadata:\n  origin: example\n---\n' + body;
    const file = new File([text.replace(/\n/g, '\r\n')], 'skill.md');
    await expect(readSkillMarkdown(file)).resolves.toEqual({
        title: 'candidate-review', summary: 'Inspect the candidate before review.', content: body,
    });
});

it.each([
    ['missing frontmatter', '# Instructions'],
    ['duplicate name', '---\nname: candidate-review\nname: override\ndescription: Review\n---\nText'],
    ['alias in optional metadata', '---\nname: candidate-review\ndescription: &review Review\nmetadata: *review\n---\nText'],
    ['invalid slug', '---\nname: candidate--review\ndescription: Review\n---\nText'],
    ['empty description', '---\nname: candidate-review\ndescription: " "\n---\nText'],
    ['YAML-escaped NUL', '---\nname: candidate-review\ndescription: "Review\\0"\n---\nText'],
    ['empty Markdown', '---\nname: candidate-review\ndescription: Review\n---\n  \n'],
])('rejects %s', async (_case, text) => {
    await expect(readSkillMarkdown(new File([text], 'SKILL.md'))).rejects.toMatchObject({ui_key: 'knowledge.import_invalid'});
});

it('rejects the wrong filename and overlarge file before reading', async () => {
    const wrong = new File(['# Instructions'], 'notes.md');
    const large = new File(['x'.repeat(32 * 1024 + 1)], 'SKILL.md');
    const wrongRead = vi.spyOn(wrong, 'arrayBuffer');
    const largeRead = vi.spyOn(large, 'arrayBuffer');
    await expect(readSkillMarkdown(wrong)).rejects.toMatchObject({ui_key: 'knowledge.import_invalid'});
    await expect(readSkillMarkdown(large)).rejects.toMatchObject({ui_key: 'knowledge.import_too_large'});
    expect(wrongRead).not.toHaveBeenCalled();
    expect(largeRead).not.toHaveBeenCalled();
});

it('checks decoded fields against the server byte limits without truncating', async () => {
    const summary = '🙂'.repeat(250);
    const content = '🙂'.repeat(4000);
    await expect(readSkillMarkdown(skill(summary, content))).resolves.toEqual({title: 'candidate-review', summary, content});
    await expect(readSkillMarkdown(skill(summary + 'a', content))).rejects.toMatchObject({ui_key: 'knowledge.import_too_large'});
    await expect(readSkillMarkdown(skill(summary, content + 'a'))).rejects.toMatchObject({ui_key: 'knowledge.import_too_large'});
});

it('distinguishes malformed UTF-8 from an unreadable local file', async () => {
    const malformed = new File([new Uint8Array([0xff])], 'SKILL.md');
    await expect(readSkillMarkdown(malformed)).rejects.toMatchObject({ui_key: 'knowledge.import_invalid'});
    const unreadable = skill();
    vi.spyOn(unreadable, 'arrayBuffer').mockRejectedValueOnce(new Error('Unavailable'));
    await expect(readSkillMarkdown(unreadable)).rejects.toMatchObject({ui_key: 'knowledge.import_read_failed'});
});
