# 文件上传 API 设计方案

## 概述

为应用端提供文件上传能力，支持 Agent 管理文件、课程资料上传等场景。

## 设计方案对比

### 方案 1: 直接 Base64 上传（推荐用于小文件）

#### 优点
- 实现简单
- 无需额外存储服务
- 可直接嵌入 JSON 请求
- 适合小文件（< 10MB）

#### 缺点
- Base64 编码增加 ~33% 体积
- 不适合大文件上传
- 内存占用高

#### 实现
```go
// POST /v1/files:upload
{
  "input": {
    "filename": "course_material.pdf",
    "content_type": "application/pdf",
    "content_base64": "SGVsbG8g...",
    "metadata": {
      "course_code": "COMP1011",
      "campus": "shenzhen"
    }
  }
}

// Response
{
  "ok": true,
  "output": {
    "file_id": "f123456",
    "size": 1024000,
    "url": "/files/f123456",
    "created_at": "2025-03-14T10:00:00Z"
  }
}
```

---

### 方案 2: 分块上传 + Filesystem MCP（推荐用于大文件）

#### 优点
- 支持大文件上传（> 100MB）
- 断点续传
- 更好的内存管理
- 集成 Filesystem MCP

#### 缺点
- 实现复杂度高
- 需要状态管理
- 需要更多的 API 端点

#### 实现
```go
// 1. 初始化上传
POST /v1/files:init_upload
{
  "input": {
    "filename": "large_file.pdf",
    "content_type": "application/pdf",
    "size": 104857600,
    "chunk_size": 5242880
  }
}

// Response
{
  "ok": true,
  "output": {
    "upload_id": "up_abc123",
    "chunk_size": 5242880,
    "total_chunks": 20
  }
}

// 2. 上传分块
POST /v1/files:upload_chunk
{
  "input": {
    "upload_id": "up_abc123",
    "chunk_index": 0,
    "content_base64": "SGVsbG8g..."
  }
}

// Response
{
  "ok": true,
  "output": {
    "chunk_index": 0,
    "uploaded": false
    "progress": 5
  }
}

// 3. 完成上传
POST /v1/files:complete_upload
{
  "input": {
    "upload_id": "up_abc123"
  }
}

// Response
{
  "ok": true,
  "output": {
    "file_id": "f123456",
    "size": 104857600,
    "url": "/files/f123456",
    "created_at": "2025-03-14T10:00:00Z"
  }
}
```

---

### 方案 3: 签名上传 URL（推荐用于超大文件）

#### 优点
- 最小化数据传输
- 适合超大文件（> 500MB）
- 客户端直接上传到存储
- 降低服务器负载

#### 缺点
- 实现复杂
- 需要额外的存储服务集成（如 S3、OSS）
- 需要处理 CORS

#### 实现
```go
// 1. 请求上传 URL
POST /v1/files:get_upload_url
{
  "input": {
    "filename": "huge_video.mp4",
    "content_type": "video/mp4",
    "size": 1073741824
  }
}

// Response
{
  "ok": true,
  "output": {
    "upload_url": "https://storage.example.com/upload/xyz123",
    "upload_id": "up_xyz123",
    "headers": {
      "Authorization": "Bearer token",
      "X-File-Name": "huge_video.mp4"
    },
    "expires_at": "2025-03-14T10:10:00Z"
  }
}

// 2. 客户端直接上传到 upload_url

// 3. 通知上传完成
POST /v1/files:upload_complete
{
  "input": {
    "upload_id": "up_xyz123",
    "storage_key": "storage/path/file.mp4"
  }
}

// Response
{
  "ok": true,
  "output": {
    "file_id": "f123456",
    "size": 1073741824,
    "url": "/files/f123456",
    "created_at": "2025-03-14T10:00:00Z"
  }
}
```

---

## 推荐实现方案

### Phase 1: 方案 1（Base64）- 快速实现

优先级：高
工作量：2-3 小时
适用场景：
- 小文件上传（< 10MB）
- 课程资料上传
- 作业提交
- 讲义资料

实现技能：
```go
// internal/skills/files_upload_skills.go

// 1. files.upload (直接 Base64 上传)
func NewFilesUploadSkill(storage *storage.Storage) Skill {
    return Skill{
        Name:    "files.upload",
        IsAsync: false,
        Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
            // 验证输入
            filename, _ := input["filename"].(string)
            contentBase64, _ := input["content_base64"].(string)
            contentType, _ := input["content_type"].(string)
            
            // 保存文件
            fileID, err := storage.SaveFile(ctx, filename, contentBase64, contentType)
            if err != nil {
                return nil, &InvokeError{
                    Code:    "STORAGE_ERROR",
                    Message: err.Error(),
                    Retryable: false,
                }
            }
            
            return map[string]any{
                "file_id": fileID,
                "size":      len(contentBase64) * 3 / 4, // 近似大小
                "url":       "/files/" + fileID,
                "created_at": time.Now().Format(time.RFC3339),
            }, nil
        },
    }
}

// 2. files.delete (删除文件)
func NewFilesDeleteSkill(storage *storage.Storage) Skill {
    return Skill{
        Name:    "files.delete",
        IsAsync: false,
        Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
            fileID, _ := input["file_id"].(string)
            
            err := storage.DeleteFile(ctx, fileID)
            if err != nil {
                return nil, &InvokeError{
                    Code:    "NOT_FOUND",
                    Message: err.Error(),
                    Retryable: false,
                }
            }
            
            return map[string]any{
                "deleted": true,
            "file_id": fileID,
            "deleted_at": time.Now().Format(time.RFC3339),
            }, nil
        },
    }
}

// 3. files.list (列出文件)
func NewFilesListSkill(storage *storage.Storage) Skill {
    return Skill{
        Name:    "files.list",
        IsAsync: false,
        Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
            fileID, _ := input["file_id"].(string)
            file, err := storage.GetFile(ctx, fileID)
            if err != nil {
                return nil, &InvokeError{
                    Code:    "NOT_FOUND",
                    Message: err.Error(),
                    Retryable: false,
                }
            }
            
            return map[string]any{
                "file": file,
            }, nil
        },
    }
}
```

---

### Phase 2: 方案 2（分块上传）- 进阶实现

优先级：中
工作量：1-2 天
适用场景：
- 大文件上传（> 10MB）
- 视频上传
- 压缩包上传

---

### Phase 3: 方案 3（签名 URL）- 高级实现

优先级：低
工作量：3-5 天
适用场景：
- 超大文件（> 500MB）
- 高频上传场景

---

## 存储层设计

```go
// internal/storage/filesystem_storage.go

package storage

import (
    "context"
    "encoding/base64"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "time"
)

type FileMetadata struct {
    ID           string
    Filename     string
    ContentType  string
    Size         int64
    CreatedAt    time.Time
    ExpiresAt    *time.Time
    StoragePath  string
}

type FilesystemStorage struct {
    baseDir string
}

func NewFilesystemStorage(baseDir string) *FilesystemStorage {
    return &FilesystemStorage{
        baseDir: baseDir,
    }
}

func (s *FilesystemStorage) SaveFile(
    ctx context.Context,
    filename string,
    contentBase64 string,
    contentType string,
) (string, error) {
    // 解码 Base64
    content, err := base64.StdEncoding.DecodeString(contentBase64)
    if err != nil {
        return "", fmt.Errorf("decode base64: %w", err)
    }
    
    // 生成文件 ID
    fileID := generateFileID()
    
    // 构建存储路径
    storagePath := filepath.Join(s.baseDir, fileID[:2], fileID[2:4])
    if err := os.MkdirAll(storagePath, 0755); err != nil {
        return "", fmt.Errorf("mkdir: %w", err)
    }
    
    filePath := filepath.Join(storagePath, fileID)
    
    // 写入文件
    if err := os.WriteFile(filePath, content, 0644); err != nil {
        return "", fmt.Errorf("write file: %w", err)
    }
    
    return fileID, nil
}

func (s *FilesystemStorage) GetFile(ctx context.Context, fileID string) (*FileMetadata, error) {
    filePath := filepath.Join(s.baseDir, fileID[:2], fileID[2:4], fileID)
    
    info, err := os.Stat(filePath)
    if err != nil {
        return nil, fmt.Errorf("stat file: %w", err)
    }
    
    return &FileMetadata{
        ID:          fileID,
        Filename:    filepath.Base(filePath),
        ContentType: guessContentType(filePath),
        Size:        info.Size(),
        CreatedAt:   info.ModTime(),
        StoragePath: filePath,
    }, nil
}

func (s *FilesystemStorage) DeleteFile(ctx context.Context, fileID string) error {
    filePath := filepath.Join(s.baseDir, fileID[:2], fileID[2:4], fileID)
    return os.Remove(filePath)
}

func (s *FilesystemStorage) ListFiles(ctx context.Context, prefix string) ([]*FileMetadata, error) {
    storagePath := filepath.Join(s.baseDir, prefix[:2], prefix[2:4])
    
    entries, err := os.ReadDir(storagePath)
    if err != nil {
        return nil, err
    }
    
    var files []*FileMetadata
    for _, entry := range entries {
        info, err := entry.Info()
        if err != nil {
            continue
        }
        
        files = append(files, &FileMetadata{
            ID:          entry.Name(),
            Filename:    entry.Name(),
            ContentType: guessContentType(entry.Name()),
            Size:        info.Size(),
            CreatedAt:   info.ModTime(),
            StoragePath: filepath.Join(storagePath, entry.Name()),
        })
    }
    
    return files, nil
}

func generateFileID() string {
    return fmt.Sprintf("%d", time.Now().UnixNano())
}

func guessContentType(filename string) string {
    ext := filepath.Ext(filename)
    switch ext {
    case ".pdf":
        return "application/pdf"
    case ".jpg", ".jpeg":
        return "image/jpeg"
    case ".png":
        return "image/png"
    case ".mp4":
        return "video/mp4"
    default:
        return "application/octet-stream"
    }
}
```

---

## 安全考虑

### 1. 文件类型验证
```go
// 只允许上传的文件类型
var allowedContentTypes = map[string]bool{
    "application/pdf":            true,
    "image/jpeg":               true,
    "image/png":                true,
    "image/gif":                true,
    "video/mp4":               true,
    "video/quicktime":          true,
    "application/zip":            true,
    "application/x-rar-compressed": true,
}
```

### 2. 文件大小限制
```go
const (
    MaxFileSizeBase64    = 10 * 1024 * 1024 // 10MB
    MaxFileSizeChunked  = 100 * 1024 * 1024 // 100MB
    MaxFileSizeSigned   = 500 * 1024 * 1024 // 500MB
)
```

### 3. 文件名安全
```go
import (
    "path/filepath"
    "regexp"
    "strings"
)

// 清理文件名，防止路径遍历
func sanitizeFilename(filename string) string {
    // 移除路径遍历字符
    filename = strings.ReplaceAll(filename, "../", "")
    filename = strings.ReplaceAll(filename, "./", "")
    filename = strings.ReplaceAll(filename, "/", "")
    
    // 移除危险字符
    filename = regexp.MustCompile(`[<>:"|?*]`).ReplaceAllString(filename, "")
    
    return filename
}
```

### 4. 病毒扫描（可选）
```go
import "github.com/dchest/staurashima/virustashima"

func scanFile(filePath string) (bool, error) {
    result, err := virustashima.ScanFile(filePath, virustashima.ScanOptions{})
    if err != nil {
        return false, err
    }
    return result.Status == virustashima.StatusClean, nil
}
```

---

## 使用示例

### 1. 上传课程资料

```bash
curl -X POST http://localhost:8080/v1/skills/files.upload:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "filename": "COMP1011_lecture.pdf",
      "content_type": "application/pdf",
      "content_base64": "SGVsbG8g...",
      "metadata": {
        "course_code": "COMP1011",
        "campus": "shenzhen",
        "file_type": "course_material"
      }
    }
  }'
```

### 2. 上传作业提交

```bash
curl -X POST http://localhost:8080/v1/skills/files.upload:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "filename": "homework1.pdf",
      "content_type": "application/pdf",
      "content_base64": "SGVsbG8g...",
      "metadata": {
        "course_code": "COMP1011",
        "assignment_id": "hw_123456",
        "file_type": "homework"
      }
    }
  }'
```

### 3. 上传图片（头像、附件）

```bash
curl -X POST http://localhost:8080/v1/skills/files.upload:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "filename": "avatar.jpg",
      "content_type": "image/jpeg",
      "content_base64": "/9j/4AAQSkZJRgABAQEAYABgAD/2wBDAyMjA4MABgAFAf/2gP/...",
      "metadata": {
        "file_type": "avatar"
      }
    }
  }'
```

### 4. 列出文件

```bash
curl -X POST http://localhost:8080/v1/skills/files.list:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "prefix": "COMP1011"
    }
  }'
```

### 5. 删除文件

```bash
curl -X POST http://localhost:8080/v1/skills/files.delete:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "file_id": "f123456"
    }
  }'
```

---

## 端侧集成示例

### JavaScript/TypeScript

```typescript
// 上传文件
async function uploadFile(file: File, metadata: any) {
  // 转换为 Base64
  const contentBase64 = await toBase64(file);
  
  const result = await invoke('files.upload', {
    filename: file.name,
    content_type: file.type,
    content_base64: contentBase64,
    metadata: metadata
  });
  
  return {
    file_id: result.file_id,
    url: result.url
  };
}

async function toBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}
```

### React Native

```typescript
import { launchImageLibrary } from 'react-native-image-picker';

async function uploadFile(file: any, metadata: any) {
  // 选择文件
  const result = await launchImageLibrary();
  const fileData = result.assets?.[0];
  
  if (!fileData) {
    throw new Error('No file selected');
  }
  
  // 读取文件为 Base64
  const base64 = await RNFS.readFile(fileData.uri, 'base64');
  
  // 上传到 agent-backend
  const uploadResult = await fetch('http://localhost:8080/v1/skills/files.upload:invoke', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      input: {
        filename: fileData.fileName,
        content_type: fileData.type,
        content_base64: base64,
        metadata: metadata
      }
    }),
  });
  
  const response = await uploadResult.json();
  if (!response.ok) {
    throw new Error(response.error.message);
  }
  
  return response.output;
}
```

---

## 错误处理

| 错误码 | 说明 | 重试 |
|--------|------|------|
| INVALID_INPUT | 缺少必需参数 | false |
| FILE_TOO_LARGE | 文件超过大小限制 | false |
| INVALID_CONTENT_TYPE | 不支持的文件类型 | false |
| STORAGE_ERROR | 存储错误 | true |
| FILE_NOT_FOUND | 文件不存在 | false |
| QUOTA_EXCEEDED | 存储配额超限 | false |

---

## 环境变量配置

```bash
# 文件存储配置
export FILE_STORAGE_PATH="/path/to/storage"
export FILE_MAX_SIZE_MB=10
export FILE_MAX_SIZE_CHUNKED_MB=100
export FILE_MAX_SIZE_SIGNED_MB=500

# 文件类型白名单
export FILE_ALLOWED_TYPES="application/pdf,image/jpeg,image/png,video/mp4"

# 病毒扫描（可选）
export ENABLE_VIRUS_SCAN=false
```

---

## 实现优先级

### Phase 1（立即实现）
✅ files.upload - Base64 上传
✅ files.delete - 删除文件
✅ files.list - 列出文件
✅ 基础存储层（Filesystem）

### Phase 2（进阶）
🔄 files.init_upload - 初始化分块上传
🔄 files.upload_chunk - 上传分块
🔄 files.complete_upload - 完成上传

### Phase 3（高级）
⏳ files.get_upload_url - 获取签名上传 URL
⏳ files.upload_complete - 通知上传完成

---

## 总结

**推荐方案：Phase 1（Base64）**
- 简单快速实现
- 适合大多数应用场景
- 可以逐步扩展到分块上传

**下一步：**
1. 实现文件存储层
2. 实现文件管理技能
3. 添加文件大小和类型验证
4. 集成 Filesystem MCP 用于文件访问

**需要我帮你实现 Phase 1 的代码吗？**
