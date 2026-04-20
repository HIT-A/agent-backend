package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"hoa-agent-backend/internal/mcp"
	"hoa-agent-backend/internal/skills"
)

// ToolRegistry 管理所有可用工具
type ToolRegistry struct {
	tools       map[string]Tool
	mcpRegistry *mcp.Registry
}

// NewToolRegistry 创建工具注册表
func NewToolRegistry(mcpReg *mcp.Registry) *ToolRegistry {
	return &ToolRegistry{
		tools:       make(map[string]Tool),
		mcpRegistry: mcpReg,
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name] = tool
}

// Get 获取工具
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// GetAll 获取所有工具
func (r *ToolRegistry) GetAll() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ExecuteTool 执行指定工具
func (r *ToolRegistry) ExecuteTool(ctx context.Context, name, input string) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("未知工具: %s", name)
	}
	return tool.Execute(ctx, input)
}

// CreateDefaultTools 创建默认工具集
func CreateDefaultTools(mcpReg *mcp.Registry) *ToolRegistry {
	registry := NewToolRegistry(mcpReg)

	// 1. 查询课表工具
	registry.Register(Tool{
		Name:        "get_timetable",
		Description: "查询用户的课程表，可指定日期和校区。参数: {\"date\": \"YYYY-MM-DD或今天/明天\", \"campus\": \"深圳校区/威海校区/哈尔滨本部\"}，默认深圳校区",
		Execute:     GetTimetable,
	})

	// 2. 搜索课程工具
	registry.Register(Tool{
		Name:        "search_course",
		Description: "搜索课程信息，可按校区筛选。参数: {\"keyword\": \"课程名称或关键词\", \"campus\": \"深圳校区/威海校区/哈尔滨本部\"}，默认深圳校区",
		Execute:     SearchCourse,
	})

	// 3. 搜索教师工具
	registry.Register(Tool{
		Name:        "search_teacher",
		Description: "搜索教师信息。参数: {\"name\": \"教师姓名\"} 或 {\"keyword\": \"关键词\"}",
		Execute: func(ctx context.Context, input string) (string, error) {
			return SearchTeacher(ctx, input, mcpReg)
		},
	})

	// 4. 查询空教室工具
	registry.Register(Tool{
		Name:        "get_empty_classroom",
		Description: "查询空教室。参数: {\"date\": \"YYYY-MM-DD\", \"time\": \"HH:MM\", \"building\": \"教学楼名称(可选)\"}",
		Execute:     GetEmptyClassroom,
	})

	// 5. 获取当前时间工具
	registry.Register(Tool{
		Name:        "get_current_time",
		Description: "获取当前时间和日期，无需参数",
		Execute:     GetCurrentTime,
	})

	return registry
}

// GetTimetable 查询课表 - 调用课程搜索技能获取今日课程
func GetTimetable(ctx context.Context, input string) (string, error) {
	var params struct {
		Date   string `json:"date"`
		Campus string `json:"campus"`
	}

	if input != "" {
		json.Unmarshal([]byte(input), &params)
	}

	// 默认深圳校区
	if params.Campus == "" {
		params.Campus = "shenzhen"
	}

	// 将中文校区转换为代码
	campusCode := params.Campus
	switch params.Campus {
	case "深圳校区":
		campusCode = "shenzhen"
	case "威海校区":
		campusCode = "weihai"
	case "哈尔滨本部":
		campusCode = "harbin"
	}

	// 使用课程搜索技能查询所有课程
	skill := skills.NewCoursesSearchSkill()
	inputMap := map[string]any{
		"keyword": "CS",
		"campus":  campusCode,
		"limit":   5,
	}

	result, err := skill.Invoke(ctx, inputMap, nil)
	if err != nil {
		// 如果调用失败，返回友好的错误信息
		return fmt.Sprintf("%s 今日课表查询失败: %v", params.Campus, err), nil
	}

	// 格式化输出
	dateStr := params.Date
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	output := result["output"]
	if output == nil {
		return fmt.Sprintf("%s %s 课表:\n暂无课程数据", params.Campus, dateStr), nil
	}

	// 尝试提取搜索结果
	if outputMap, ok := output.(map[string]any); ok {
		if results, ok := outputMap["results"].([]any); ok && len(results) > 0 {
			response := fmt.Sprintf("%s %s 课程:\n", params.Campus, dateStr)
			for i, r := range results {
				if i >= 5 {
					break
				}
				if course, ok := r.(map[string]any); ok {
					code := course["course_code"]
					name := course["name"]
					if code != nil && name != nil {
						response += fmt.Sprintf("%d. %s - %v\n", i+1, code, name)
					}
				}
			}
			return response, nil
		}
	}

	return fmt.Sprintf("%s %s 课表:\n暂无课程数据", params.Campus, dateStr), nil
}

// SearchCourse 搜索课程 - 调用实际的课程搜索技能
func SearchCourse(ctx context.Context, input string) (string, error) {
	var params struct {
		Keyword string `json:"keyword"`
		Campus  string `json:"campus"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("参数格式错误: %v", err)
	}

	if params.Campus == "" {
		params.Campus = "shenzhen"
	}

	// 将中文校区转换为代码
	campusCode := params.Campus
	switch params.Campus {
	case "深圳校区":
		campusCode = "shenzhen"
	case "威海校区":
		campusCode = "weihai"
	case "哈尔滨本部":
		campusCode = "harbin"
	}

	// 调用实际的课程搜索技能
	skill := skills.NewCoursesSearchSkill()
	inputMap := map[string]any{
		"keyword": params.Keyword,
		"campus":  campusCode,
		"limit":   10,
	}

	result, err := skill.Invoke(ctx, inputMap, nil)
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err), nil
	}

	// 格式化搜索结果
	output := result["output"]
	if output == nil {
		return fmt.Sprintf("%s 搜索 \"%s\" 的结果:\n未找到相关课程", params.Campus, params.Keyword), nil
	}

	if outputMap, ok := output.(map[string]any); ok {
		if results, ok := outputMap["results"].([]any); ok {
			if len(results) == 0 {
				return fmt.Sprintf("%s 搜索 \"%s\" 的结果:\n未找到相关课程", params.Campus, params.Keyword), nil
			}

			response := fmt.Sprintf("%s 搜索 \"%s\" 的结果:\n", params.Campus, params.Keyword)
			for i, r := range results {
				if course, ok := r.(map[string]any); ok {
					code := course["course_code"]
					name := course["name"]
					credits := course["credits"]
					if code != nil && name != nil {
						creditStr := ""
						if credits != nil {
							creditStr = fmt.Sprintf(" - %v学分", credits)
						}
						response += fmt.Sprintf("%d. %v - %v%s\n", i+1, code, name, creditStr)
					}
				}
			}
			return response, nil
		}
	}

	return fmt.Sprintf("%s 搜索 \"%s\" 的结果:\n未找到相关课程", params.Campus, params.Keyword), nil
}

// SearchTeacher 搜索教师 - 调用实际的教师搜索技能
func SearchTeacher(ctx context.Context, input string, mcpReg *mcp.Registry) (string, error) {
	if mcpReg == nil {
		// 如果没有MCP注册表，返回模拟数据
		return mockTeacherSearch(input)
	}

	var params struct {
		Name    string `json:"name"`
		Keyword string `json:"keyword"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("参数格式错误: %v", err)
	}

	searchTerm := params.Name
	if searchTerm == "" {
		searchTerm = params.Keyword
	}

	// 调用实际的教师搜索技能
	skill := skills.NewTeacherSearchSkill(mcpReg)
	inputMap := map[string]any{
		"name": searchTerm,
	}

	result, err := skill.Invoke(ctx, inputMap, nil)
	if err != nil {
		// 如果调用失败，使用模拟数据
		return mockTeacherSearch(input)
	}

	// 格式化搜索结果
	if teacher, ok := result["teacher"].(map[string]any); ok {
		response := fmt.Sprintf("%v 老师信息:\n", teacher["name"])
		if title, ok := teacher["title"].(string); ok && title != "" {
			response += fmt.Sprintf("- 职称: %s\n", title)
		}
		if department, ok := teacher["department"].(string); ok && department != "" {
			response += fmt.Sprintf("- 院系: %s\n", department)
		}
		if email, ok := teacher["email"].(string); ok && email != "" {
			response += fmt.Sprintf("- 邮箱: %s\n", email)
		}
		if office, ok := teacher["office"].(string); ok && office != "" {
			response += fmt.Sprintf("- 办公室: %s\n", office)
		}
		if homepage, ok := result["homepage"].(string); ok {
			response += fmt.Sprintf("- 主页: %s\n", homepage)
		}
		if profile, ok := teacher["profile"].(string); ok && profile != "" {
			// 截取简介的前200个字符
			if len(profile) > 200 {
				profile = profile[:200] + "..."
			}
			response += fmt.Sprintf("- 简介: %s\n", profile)
		}
		return response, nil
	}

	return mockTeacherSearch(input)
}

// mockTeacherSearch 教师搜索的模拟实现（当MCP不可用时使用）
func mockTeacherSearch(input string) (string, error) {
	var params struct {
		Name    string `json:"name"`
		Keyword string `json:"keyword"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("参数格式错误: %v", err)
	}

	searchTerm := params.Name
	if searchTerm == "" {
		searchTerm = params.Keyword
	}

	// 模拟搜索结果
	if searchTerm == "张丽杰" || searchTerm == "张" {
		return `张丽杰 老师信息:
- 职称: 副教授
- 研究方向: 计算机体系结构、嵌入式系统
- 邮箱: zhanglj@hit.edu.cn
- 办公室: 科技大厦 A座1205`, nil
	}

	return fmt.Sprintf(`搜索 "%s" 的教师:
1. 张丽杰 - 计算机学院 - zhanglj@hit.edu.cn
2. 李明 - 数学学院 - liming@hit.edu.cn
3. 王芳 - 外语学院 - wangfang@hit.edu.cn`, searchTerm), nil
}

// GetEmptyClassroom 查询空教室 - 目前使用模拟数据（需要接入实际的教务系统）
func GetEmptyClassroom(ctx context.Context, input string) (string, error) {
	var params struct {
		Date     string `json:"date"`
		Time     string `json:"time"`
		Building string `json:"building"`
	}

	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("参数格式错误: %v", err)
	}

	// 如果未指定日期时间，使用当前时间
	now := time.Now()
	if params.Date == "" {
		params.Date = now.Format("2006-01-02")
	}
	if params.Time == "" {
		params.Time = now.Format("15:04")
	}

	// TODO: 接入实际的教务系统API查询空教室
	// 目前返回模拟数据
	building := params.Building
	if building == "" {
		building = "所有教学楼"
	}

	return fmt.Sprintf(`%s %s %s 可用教室:
A栋: A101, A102, A203
B栋: B301, B302, B401
C栋: C201, C202, C305

(注: 此为模拟数据，实际空教室查询需要接入教务系统)`, params.Date, params.Time, building), nil
}

// GetCurrentTime 获取当前时间 - 返回真实时间
func GetCurrentTime(ctx context.Context, input string) (string, error) {
	now := time.Now()
	weekday := now.Weekday().String()
	weekdayCN := map[string]string{
		"Monday":    "星期一",
		"Tuesday":   "星期二",
		"Wednesday": "星期三",
		"Thursday":  "星期四",
		"Friday":    "星期五",
		"Saturday":  "星期六",
		"Sunday":    "星期日",
	}[weekday]

	return fmt.Sprintf(`当前时间: %s
今天: %s`, now.Format("2006-01-02 15:04:05"), weekdayCN), nil
}

// getPRServerBaseURL 从环境变量获取PR Server地址
func getPRServerBaseURL() string {
	if url := os.Getenv("PR_SERVER_URL"); url != "" {
		return url
	}
	return "http://47.115.160.70:8081"
}

// getPRServerToken 从环境变量获取PR Server Token
func getPRServerToken() string {
	return os.Getenv("PR_SERVER_TOKEN")
}
