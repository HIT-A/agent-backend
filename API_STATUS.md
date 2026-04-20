# API 可用性状态

## 测试时间
2025-04-08

## 服务状态
- Agent Backend: ✅ 运行中 (localhost:8080)
- PR Server: ✅ 已连接 (47.115.160.70:8081)
- COS: ⚠️ 已配置 (需要验证bucket访问)
- Qdrant: ⚠️ 未启动 (docker-compose up -d)
- MCP服务: ⚠️ 未配置 (crawl4ai等)

## API 测试结果

| API | 状态 | 说明 |
|-----|------|------|
| GET /health | ✅ | {"status":"ok"} |
| GET /v1/skills | ✅ | 31个技能 |
| POST /api/courses/search | ✅ | 正常返回课程列表 |
| POST /api/courses/read | ⚠️ | 依赖PR Server，理论上可用 |
| POST /api/pr/* | ⚠️ | 依赖PR Server，理论上可用 |
| POST /api/files/* | ⚠️ | COS已配置，需要测试bucket访问 |
| POST /api/hitsz/login | ⚠️ | Go重写完成，需要调试SSO流程 |
| POST /api/rag/* | ❌ | 需要Qdrant + Embedding API |
| POST /api/crawl/* | ❌ | 需要MCP服务 |
| POST /api/teachers/* | ❌ | 需要MCP服务 |

## 核心功能
- ✅ 课表查询已可用
- ✅ RESTful API框架完成
- ✅ 兼容旧版Skill API

## 待完善
1. HITSZ SSO登录调试
2. COS文件操作测试
3. RAG向量搜索（启动Qdrant）
4. MCP服务配置（如需爬虫功能）
