# RAG Data Repository Structure

## 目录结构设计

```
HITA_RagData/
├── README.md                    # 仓库说明
├── sources/                     # 按来源组织的原始数据
│   ├── github/                  # GitHub 仓库来源
│   │   ├── hitlug-hit-network-resource/
│   │   │   ├── metadata.json    # 元数据（来源、时间、文件数等）
│   │   │   ├── raw/             # 原始文件
│   │   │   │   ├── file1.pdf
│   │   │   │   └── file2.docx
│   │   │   └── converted/       # 转换后的 Markdown
│   │   │       ├── file1.md
│   │   │       └── file2.md
│   │   ├── hithesis-hithesis/
│   │   └── ...
│   ├── crawled/                 # 爬虫抓取来源
│   │   ├── python-docs/
│   │   │   ├── metadata.json
│   │   │   └── pages/
│   │   │       ├── page1.md
│   │   │       └── page2.md
│   │   └── ...
│   └── manual/                  # 手动上传来源
│       └── ...
├── index/                       # 索引文件（用于 RAG）
│   ├── documents.json           # 文档索引
│   └── chunks.json              # 分块索引
└── scripts/                     # 处理脚本
    └── sync.sh                  # 同步脚本
```

## 元数据格式 (metadata.json)

```json
{
  "source": {
    "type": "github",
    "repo": "hitlug/hit-network-resource",
    "ref": "main",
    "url": "https://github.com/hitlug/hit-network-resource"
  },
  "sync": {
    "timestamp": "2025-03-15T12:00:00Z",
    "version": "abc123",
    "files_total": 10,
    "files_processed": 10,
    "chunks_total": 31
  },
  "files": [
    {
      "original": "docs/README.md",
      "converted": "converted/README.md",
      "size": 2048,
      "chunks": 3
    }
  ]
}
```

## 文档索引格式 (documents.json)

```json
[
  {
    "id": "hitlug-hit-network-resource-readme",
    "source": "github/hitlug-hit-network-resource",
    "title": "README",
    "path": "sources/github/hitlug-hit-network-resource/converted/README.md",
    "cos_url": "https://cos.xxx/rag-content/...",
    "chunks": 3,
    "updated_at": "2025-03-15T12:00:00Z"
  }
]
```

## 工作流程

1. **下载** → 从 GitHub/爬虫获取原始文件
2. **转换** → 统一转为 Markdown
3. **整理** → 按目录结构存放
4. **存储 COS** → 上传到 COS
5. **更新索引** → 更新 documents.json
6. **Git 提交** → commit + push 到 HIT-A/HITA_RagData
7. **RAG 摄入** → 向量化存入 Qdrant