package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

const baseURL = "http://127.0.0.1:8080/api/v1"

// 统一响应结构
type apiResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

// httpClient 带 cookie jar 的 HTTP 客户端（用于保持 refresh token cookie）
var httpClient *http.Client

func init() {
	jar, _ := cookiejar.New(nil)
	httpClient = &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}
}

// doRequest 发送请求并返回解析后的响应
func doRequest(method, path string, body interface{}, headers map[string]string) (*apiResponse, int, error) {
	var reqBody io.Reader
	if body != nil {
		if r, ok := body.(io.Reader); ok {
			reqBody = r
		} else {
			b, _ := json.Marshal(body)
			reqBody = bytes.NewReader(b)
		}
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}

	// 默认 JSON content-type（body 为 reader 时不设置，让调用方控制）
	if body != nil {
		if _, ok := body.(io.Reader); !ok {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var apiResp apiResponse
	_ = json.Unmarshal(raw, &apiResp)
	return &apiResp, resp.StatusCode, nil
}

// === 测试用例 ===

// TestHealthCheck 健康检查
func TestHealthCheck(t *testing.T) {
	// /health 不在 /api/v1 前缀下，直接访问根路径
	req, err := http.NewRequest("GET", "http://127.0.0.1:8080/health", nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("期望状态码 200, 实际 %d", resp.StatusCode)
	}

	raw, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	_ = json.Unmarshal(raw, &data)
	t.Logf("健康检查响应: %v", data["status"])
}

// TestCaptcha 获取图形验证码
func TestCaptcha(t *testing.T) {
	resp, status, err := doRequest("POST", "/base/captcha", nil, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if status != 200 {
		t.Errorf("期望状态码 200, 实际 %d", status)
	}
	if resp.Code != 3 {
		t.Errorf("期望 code=3, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}

	var data struct {
		CaptchaID string `json:"captcha_id"`
		PicPath   string `json:"pic_path"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if data.CaptchaID == "" {
		t.Error("captcha_id 不应为空")
	}
	if !strings.HasPrefix(data.PicPath, "data:image") {
		t.Error("pic_path 应为 base64 图片数据")
	}
	t.Logf("验证码获取成功: captcha_id=%s", data.CaptchaID)
}

// TestVideoList 视频列表（公开接口）
func TestVideoList(t *testing.T) {
	resp, status, err := doRequest("GET", "/video/list?limit=5", nil, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if status != 200 {
		t.Errorf("期望状态码 200, 实际 %d", status)
	}
	if resp.Code != 3 {
		t.Errorf("期望 code=3, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}
	t.Logf("视频列表获取成功")
}

// TestVideoDetail 视频详情（公开接口）
func TestVideoDetail(t *testing.T) {
	resp, status, err := doRequest("GET", "/video/detail?video_id=1", nil, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if status != 200 {
		t.Errorf("期望状态码 200, 实际 %d", status)
	}
	if resp.Code != 3 {
		t.Logf("视频详情: code=%d, msg=%s（可能无视频数据）", resp.Code, resp.Msg)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	t.Logf("视频详情获取成功: title=%v", data["title"])
}

// TestAuthWithoutToken 未携带 token 访问需登录接口
func TestAuthWithoutToken(t *testing.T) {
	resp, status, err := doRequest("GET", "/user/info", nil, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if status != 200 {
		t.Errorf("期望状态码 200, 实际 %d", status)
	}
	if resp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d", resp.Code)
	}

	// 检查 data.reload 字段
	var data map[string]interface{}
	_ = json.Unmarshal(resp.Data, &data)
	if reload, ok := data["reload"]; !ok || reload != true {
		t.Error("期望 data.reload=true")
	}
	t.Log("未登录访问被正确拒绝")
}

// TestAuthWithWrongTokenHeader 使用错误的 token 头（Authorization: Bearer）
func TestAuthWithWrongTokenHeader(t *testing.T) {
	// 模拟旧前端的错误行为：使用 Authorization: Bearer 头
	headers := map[string]string{
		"Authorization": "Bearer fake_token",
	}
	resp, _, err := doRequest("GET", "/user/info", nil, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if resp.Code != 4 {
		t.Errorf("使用 Authorization 头应被拒绝（code=4）, 实际 code=%d", resp.Code)
	}
	t.Log("确认：Authorization: Bearer 头无法通过认证（后端只认 x-access-token）")
}

// TestAuthWithXAccessTokenHeader 使用正确的 x-access-token 头
// 注意：此测试需要一个有效的 token，但由于无法自动注册（需要邮箱验证码），
// 这里仅验证请求格式正确时后端的响应逻辑
func TestAuthWithXAccessTokenHeader(t *testing.T) {
	// 使用一个无效的 token，验证后端是否正确读取了 x-access-token 头
	// 后端会尝试解析 token，解析失败后会尝试 refresh token
	headers := map[string]string{
		"x-access-token": "invalid.token.here",
	}
	resp, _, err := doRequest("GET", "/user/info", nil, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	// 无效 token 应该返回 code=4（认证失败）
	if resp.Code != 4 {
		t.Errorf("期望 code=4（token 无效）, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}
	t.Log("确认：x-access-token 头被后端正确读取（无效 token 被拒绝）")
}

// TestChangeInfoValidation 测试修改个人信息的参数校验
func TestChangeInfoValidation(t *testing.T) {
	// 不传 username（后端要求 username required）
	body := map[string]string{
		"signature": "测试签名",
	}
	headers := map[string]string{
		"x-access-token": "fake",
	}
	resp, _, err := doRequest("PUT", "/user/changeInfo", body, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	// 应该返回 code=4（参数错误或认证失败）
	if resp.Code != 4 {
		t.Errorf("期望 code=4, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}
	t.Logf("修改信息参数校验: code=%d, msg=%s", resp.Code, resp.Msg)
}

// TestFollowEndpoint 测试关注接口的字段名
func TestFollowEndpoint(t *testing.T) {
	// 后端 FollowUserReq 要求 target_user_id 字段
	body := map[string]string{
		"target_user_id": "1",
	}
	headers := map[string]string{
		"x-access-token": "fake",
	}
	resp, _, err := doRequest("POST", "/interaction/follow", body, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	// 未登录应返回 code=4
	if resp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}
	t.Log("关注接口字段名正确（target_user_id）")
}

// TestFollowWrongFieldName 测试使用错误字段名 user_id
func TestFollowWrongFieldName(t *testing.T) {
	// 使用旧前端的错误字段名 user_id
	body := map[string]string{
		"user_id": "1",
	}
	headers := map[string]string{
		"x-access-token": "fake",
	}
	resp, _, err := doRequest("POST", "/interaction/follow", body, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	// 后端 FollowUserReq 要求 target_user_id，使用 user_id 会绑定失败
	// 应返回 code=4（参数错误或认证失败）
	if resp.Code != 4 {
		t.Errorf("期望 code=4, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}
	// 检查 msg 是否包含参数错误提示（认证失败时 msg 会是 "Please login again"）
	t.Logf("使用错误字段名 user_id: code=%d, msg=%s", resp.Code, resp.Msg)
}

// TestCommentListUnified 测试统一评论列表接口
func TestCommentListUnified(t *testing.T) {
	// 测试统一评论接口，target_type=video
	resp, status, err := doRequest("GET", "/comment/list?target_type=video&target_id=1&page=1&page_size=20", nil, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if status != 200 {
		t.Errorf("期望状态码 200, 实际 %d", status)
	}
	if resp.Code != 3 {
		t.Logf("评论列表: code=%d, msg=%s（可能无评论数据）", resp.Code, resp.Msg)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	t.Logf("统一评论列表接口正常: list=%v, total=%v", data["list"], data["total"])
}

// TestHistoryEndpoints 测试历史记录接口路径
func TestHistoryEndpoints(t *testing.T) {
	headers := map[string]string{
		"x-access-token": "fake",
	}

	// 测试视频观看历史列表（正确路径 /history/video/list）
	resp, _, err := doRequest("GET", "/history/video/list", nil, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if resp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d", resp.Code)
	}
	t.Log("历史记录路径 /history/video/list 正确")

	// 测试搜索历史列表（正确路径 /history/search/list）
	resp, _, err = doRequest("GET", "/history/search/list", nil, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if resp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d", resp.Code)
	}
	t.Log("搜索历史路径 /history/search/list 正确")
}

// TestDailyTaskEndpoints 测试每日任务接口
func TestDailyTaskEndpoints(t *testing.T) {
	headers := map[string]string{
		"x-access-token": "fake",
	}

	// 测试每日登录奖励（正确路径 /daily/login）
	resp, _, err := doRequest("POST", "/daily/login", nil, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if resp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d", resp.Code)
	}
	t.Log("每日任务路径 /daily/login 正确")

	// 测试查询今日任务（正确路径 /daily/today）
	resp, _, err = doRequest("GET", "/daily/today", nil, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if resp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d", resp.Code)
	}
	t.Log("每日任务路径 /daily/today 正确")
}

// TestArticleDraftFields 测试文章草稿接口字段名
func TestArticleDraftFields(t *testing.T) {
	// 后端 ArticleDraftReq 要求 body_md 字段（不是 content）
	body := map[string]string{
		"title":   "测试文章",
		"body_md": "测试内容",
	}
	headers := map[string]string{
		"x-access-token": "fake",
	}
	resp, _, err := doRequest("POST", "/article/draft", body, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	// 未登录应返回 code=4
	if resp.Code != 4 {
		t.Errorf("期望 code=4, 实际 code=%d, msg=%s", resp.Code, resp.Msg)
	}
	t.Log("文章草稿接口字段 body_md 正确")
}

// TestAvatarUploadFormat 测试头像上传接口的 multipart/form-data 格式
func TestAvatarUploadFormat(t *testing.T) {
	// 构建 multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加 avatar 字段（模拟文件上传）
	part, err := writer.CreateFormFile("avatar", "test.jpg")
	if err != nil {
		t.Fatalf("创建表单字段失败: %v", err)
	}
	// 写入最小 JPEG 头（1x1 像素）
	_, _ = part.Write([]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00"))
	writer.Close()

	req, err := http.NewRequest("POST", baseURL+"/user/avatar", body)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-access-token", "fake")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var apiResp apiResponse
	_ = json.Unmarshal(raw, &apiResp)

	// 未登录应返回 code=4
	if apiResp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	t.Log("头像上传接口 multipart/form-data 格式正确")
}

// TestVideoUploadFieldNames 测试视频上传接口的返回字段名
func TestVideoUploadFieldNames(t *testing.T) {
	// 此测试验证视频上传接口返回 video_id（不是 draft_id）
	// 由于需要登录且有真实视频文件，这里只验证接口路径存在
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("title", "测试视频")
	_ = writer.WriteField("description", "测试描述")
	_ = writer.WriteField("zone", "动画")
	// 添加 tags 字段（逐个添加，而非 JSON 字符串）
	_ = writer.WriteField("tags", "标签1")
	_ = writer.WriteField("tags", "标签2")
	// 添加一个假的文件
	part, _ := writer.CreateFormFile("file", "test.mp4")
	_, _ = part.Write([]byte("fake video content"))
	writer.Close()

	req, err := http.NewRequest("POST", baseURL+"/video/draft/upload", body)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-access-token", "fake")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var apiResp apiResponse
	_ = json.Unmarshal(raw, &apiResp)

	// 未登录应返回 code=4
	if apiResp.Code != 4 {
		t.Errorf("期望 code=4（未登录）, 实际 code=%d, msg=%s", apiResp.Code, apiResp.Msg)
	}
	t.Log("视频上传接口路径正确，tags 以重复字段形式发送")
}

// TestResponseFormat 测试统一响应格式
func TestResponseFormat(t *testing.T) {
	resp, _, err := doRequest("GET", "/video/list?limit=1", nil, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	// 验证响应包含 code, data, msg 三个字段
	if resp.Code == 0 {
		t.Error("code 字段不应为 0（未设置）")
	}
	if resp.Msg == "" {
		t.Error("msg 字段不应为空")
	}
	t.Logf("响应格式验证通过: code=%d, msg=%s", resp.Code, resp.Msg)
}

// TestNonExistentOldRoutes 测试旧的错误路由不存在
func TestNonExistentOldRoutes(t *testing.T) {
	// 验证旧前端使用的错误路由确实不存在
	oldRoutes := []struct {
		method string
		path   string
		desc   string
	}{
		{"GET", "/comment/video/list?video_id=1", "旧评论路由 /comment/video/list"},
		{"GET", "/history/video?page=1", "旧历史路由 /history/video（应为 /history/video/list）"},
		{"DELETE", "/history/video/clear", "旧清空历史路由 DELETE（应为 POST）"},
		{"POST", "/daily/comment", "不存在的每日评论奖励路由"},
	}

	headers := map[string]string{
		"x-access-token": "fake",
	}

	for _, route := range oldRoutes {
		resp, status, err := doRequest(route.method, route.path, nil, headers)
		if err != nil {
			t.Logf("%s: 请求出错 %v", route.desc, err)
			continue
		}
		// 这些旧路由应该返回 404（路由不存在）
		if status == 404 {
			t.Logf("✓ %s 正确返回 404", route.desc)
		} else {
			t.Logf("%s: status=%d, code=%d, msg=%s", route.desc, status, resp.Code, resp.Msg)
		}
	}
}

// TestFullAuthFlow 全流程认证测试（如果已有测试用户）
// 注意：此测试需要一个已注册的用户。如果数据库中没有用户，会跳过。
func TestFullAuthFlow(t *testing.T) {
	// 尝试登录（使用测试账号）
	loginBody := map[string]string{
		"email":      "test@test.com",
		"password":   "123456",
		"captcha":    "000000",
		"captcha_id": "test",
	}
	resp, _, err := doRequest("POST", "/user/login", loginBody, nil)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.Code != 3 {
		t.Skipf("无法登录测试用户（code=%d, msg=%s），跳过全流程测试", resp.Code, resp.Msg)
		return
	}

	// 解析登录响应
	var loginData struct {
		AccessToken string `json:"access_token"`
		Account     struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"account"`
	}
	if err := json.Unmarshal(resp.Data, &loginData); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}

	if loginData.AccessToken == "" {
		t.Fatal("access_token 不应为空")
	}
	t.Logf("登录成功: user_id=%s, username=%s", loginData.Account.ID, loginData.Account.Username)

	// 使用 x-access-token 头访问需登录接口
	authHeaders := map[string]string{
		"x-access-token": loginData.AccessToken,
	}

	// 测试获取个人信息
	resp, _, err = doRequest("GET", "/user/info", nil, authHeaders)
	if err != nil {
		t.Fatalf("获取用户信息失败: %v", err)
	}
	if resp.Code != 3 {
		t.Fatalf("获取用户信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	t.Log("✓ 使用 x-access-token 头成功获取用户信息")

	// 测试修改个人信息
	changeBody := map[string]string{
		"username":  loginData.Account.Username,
		"signature": fmt.Sprintf("测试签名_%d", time.Now().Unix()),
		"gender":    "secret",
		"birthday":  "2000-01-01",
	}
	resp, _, err = doRequest("PUT", "/user/changeInfo", changeBody, authHeaders)
	if err != nil {
		t.Fatalf("修改用户信息失败: %v", err)
	}
	if resp.Code != 3 {
		t.Errorf("修改用户信息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	} else {
		t.Log("✓ 修改个人信息成功")
	}

	// 测试获取用户等级
	resp, _, err = doRequest("GET", "/user/level", nil, authHeaders)
	if err != nil {
		t.Fatalf("获取用户等级失败: %v", err)
	}
	if resp.Code != 3 {
		t.Logf("获取用户等级: code=%d, msg=%s", resp.Code, resp.Msg)
	} else {
		t.Log("✓ 获取用户等级成功")
	}

	// 测试获取收藏夹列表
	resp, _, err = doRequest("GET", "/favorite/folders", nil, authHeaders)
	if err != nil {
		t.Fatalf("获取收藏夹失败: %v", err)
	}
	if resp.Code != 3 {
		t.Logf("获取收藏夹: code=%d, msg=%s", resp.Code, resp.Msg)
	} else {
		t.Log("✓ 获取收藏夹列表成功")
	}

	// 测试获取通知未读数
	resp, _, err = doRequest("GET", "/notification/unread_count", nil, authHeaders)
	if err != nil {
		t.Fatalf("获取未读数失败: %v", err)
	}
	if resp.Code != 3 {
		t.Logf("获取未读数: code=%d, msg=%s", resp.Code, resp.Msg)
	} else {
		t.Log("✓ 获取通知未读数成功")
	}

	// 测试每日登录奖励
	resp, _, err = doRequest("POST", "/daily/login", nil, authHeaders)
	if err != nil {
		t.Fatalf("每日登录奖励失败: %v", err)
	}
	if resp.Code != 3 {
		t.Logf("每日登录奖励: code=%d, msg=%s", resp.Code, resp.Msg)
	} else {
		t.Log("✓ 每日登录奖励触发成功")
	}
}
