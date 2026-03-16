# Unstructured MCP Server

Document format conversion to Markdown using Unstructured library.

## Supported Formats

| Format | Library | Notes |
|--------|---------|-------|
| `.pdf` | PyMuPDF | Fast, preserves layout |
| `.docx`/`.doc` | python-docx | .doc auto-converted to .docx |
| `.pptx`/`.ppt` | python-pptx | .ppt auto-converted to .pptx |
| `.md`/`.txt` | built-in | Pass-through |
| `.html` | unstructured | Web content |
| Others | unstructured | JSON, CSV, RTF, etc. |

## Installation

```bash
# Using Docker (recommended)
cd mcp-servers/unstructured
docker build -t unstructured-mcp .

# Local installation
pip install -r requirements.txt
```

## Tools

### convert_to_markdown
Convert a single document to Markdown.

### batch_convert
Convert multiple documents in one call.

### get_supported_formats
List supported formats and library availability.