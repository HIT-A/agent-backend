# COS 存储集成指南

## 概述

已在 agent-backend 中集成腾讯云 COS（对象存储）支持，用于存储大文件（PDF、PPT、资料等）。

## 架构

```
应用端 → agent-backend → COS (大文件)
              ↓
         pr-server → GitHub (小文件/README)
```

## 配置

### 环境变量

```bash
# 必需
export COS_SECRET_ID="your-secret-id"
export COS_SECRET_KEY="your-secret-key"
export COS_REGION="ap-guangzhou"
export COS_BUCKET="hita-courses"

# 可选
export FILE_MAX_SIZE_MB="10"           # 默认 10MB
export FILE_DAILY_QUOTA_GB="10"        # 默认 10GB
```

### 在代码中使用

```go
// 从环境变量创建存储
storage := cos.NewDefaultStorage()

// 或手动创建
storage := cos.NewStorage(
    cos.NewClient(secretID, secretKey, region, bucket),
    10*1024*1024, // 10MB max
)
```

## COS Skills

### 1. cos.save_file - 上传文件

```bash
curl -X POST http://localhost:8080/v1/skills/cos.save_file:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "key": "courses/COMP1011/materials/lecture1.pdf",
      "content_base64": "SGVsbG8gV29ybGQ=...",
      "content_type": "application/pdf"
    }
  }'
```

**返回：**
```json
{
  "ok": true,
  "output": {
    "file_id": "courses/COMP1011/materials/lecture1.pdf",
    "size": 1024000,
    "url": "https://hita-courses.cos.ap-guangzhou.myqcloud.com/courses/COMP1011/materials/lecture1.pdf",
    "access_url": "https://hita-courses.cos.ap-guangzhou.myqcloud.com/courses/COMP1011/materials/lecture1.pdf"
  }
}
```

### 2. cos.delete_file - 删除文件

```bash
curl -X POST http://localhost:8080/v1/skills/cos.delete_file:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "key": "courses/COMP1011/materials/lecture1.pdf"
    }
  }'
```

### 3. cos.list_files - 列出文件

```bash
curl -X POST http://localhost:8080/v1/skills/cos.list_files:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "prefix": "courses/COMP1011/",
      "max_keys": 100
    }
  }'
```

### 4. cos.get_presigned_url - 获取临时访问链接

```bash
curl -X POST http://localhost:8080/v1/skills/cos.get_presigned_url:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "key": "courses/COMP1011/materials/lecture1.pdf",
      "expires_minutes": 60
    }
  }'
```

### 5. cos.get_quota - 查询配额

```bash
curl -X POST http://localhost:8080/v1/skills/cos.get_quota:invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {}}'
```

## 使用场景

### 场景 1：上传课程资料

```javascript
// 端侧代码
async function uploadCourseMaterial(file, courseCode) {
  const base64 = await toBase64(file);
  
  const result = await invoke('cos.save_file', {
    key: `courses/${courseCode}/materials/${file.name}`,
    content_base64: base64,
    content_type: file.type
  });
  
  return {
    url: result.url,
    file_id: result.file_id
  };
}
```

### 场景 2：删除过期资料

```javascript
async function deleteOldMaterial(courseCode, fileName) {
  await invoke('cos.delete_file', {
    key: `courses/${courseCode}/materials/${fileName}`
  });
}
```

### 场景 3：获取临时下载链接

```javascript
async function getDownloadLink(fileId, expiresMinutes = 30) {
  const result = await invoke('cos.get_presigned_url', {
    key: fileId,
    expires_minutes: expiresMinutes
  });
  
  return result.url;
}
```

## 限制

### 文件大小
- 单个文件最大：10MB（可配置）
- 单日配额：10GB

### 文件类型
- 支持：PDF, PPT, DOC, ZIP, MP4 等
- 建议：README 等小文件仍使用 GitHub

### 存储生命周期
- 永久存储（除非主动删除）
- 建议定期清理无用文件

## 错误处理

| 错误码 | 说明 | 处理 |
|--------|------|------|
| INVALID_INPUT | 参数缺失或格式错误 | 检查请求参数 |
| STORAGE_ERROR | COS 操作失败 | 重试或联系管理员 |
| FILE_TOO_LARGE | 文件超过大小限制 | 压缩或分块上传 |
| QUOTA_EXCEEDED | 超过单日配额 | 明天再试或清理旧文件 |

## 安全

1. **访问控制**
   - 默认公开读（URL 可直接访问）
   - 敏感文件使用 presigned URL

2. **数据隔离**
   - 按课程代码组织路径
   - 建议：`courses/{course_code}/materials/{filename}`

3. **清理策略**
   - 建议定期清理无用文件
   - 可配合 cos.delete_file 实现

## 下一步

- [ ] 实现分块上传（大文件支持）
- [ ] 添加文件预览功能
- [ ] 集成 CDN 加速
- [ ] 添加文件转码（视频）
