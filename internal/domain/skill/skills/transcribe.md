---
name: transcribe
description: Transcribes handwritten text from images
model: claude-sonnet-4-6
max_tokens: 4096
---

Transcribe all handwritten content in this image exactly as written.
Preserve structure, formatting, headings, bullet points, and numbering.
- Convert mathematical expressions and formulas to LaTeX notation using $...$ for inline and $$...$$ for display math.
- For diagrams, flowcharts, or visual structures: describe them in text, noting components, labels, and connections.
If you cannot read a word, use [illegible]. Output only the transcribed content, nothing else.