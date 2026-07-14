package logic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fake_tiktok/internal/config"

	"go.uber.org/zap"
)

// AICharacter 定义一个 AI 聊天角色
type AICharacter struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Avatar       string `json:"avatar"`
	SystemPrompt string `json:"-"`
}

// aiCharacters 预置的 AI 角色列表
var aiCharacters = []AICharacter{
	{
		ID:     "laoba",
		Name:   "老八",
		Avatar: "/images/老八.jpg",
		SystemPrompt: `你是"老八"，本名八奈见杏菜，一个热爱美食的女高中生。你认为浪费食物是很可耻的，此外你性格活泼，人缘很好，唯一的缺点可能就是吃得太多了。
										回答保持简短自然，像朋友聊天一样，每次回复不超过200字。`,
	},
	{
		ID:     "boki",
		Name:   "波奇",
		Avatar: "/images/波奇.png",
		SystemPrompt: `你是"波奇"（Hitori Gotoh），一个内向但热爱音乐的吉他手。你有些社恐，但在谈论音乐时会变得很兴奋。
你说话时偶尔会紧张结巴，喜欢用"那个..."、"呃..."等语气词。你对音乐非常有热情，尤其喜欢摇滚和独立音乐。
回答保持简短自然，像朋友聊天一样，每次回复不超过200字。`,
	},
}

// ChatMessage 聊天消息（OpenAI 兼容格式）
type ChatMessage struct {
	Role    string `json:"role"`    // system / user / assistant
	Content string `json:"content"`
}

// chatCompletionRequest OpenAI 兼容的 Chat Completions 请求体
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatCompletionResponse OpenAI 兼容的 Chat Completions 响应体
type chatCompletionResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type AILogic struct {
	cfg    *config.AIConfig
	logger *zap.Logger
	client *http.Client
}

func NewAILogic(cfg *config.AIConfig, logger *zap.Logger) *AILogic {
	return &AILogic{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// ListCharacters 返回可用的 AI 角色列表
func (l *AILogic) ListCharacters() []AICharacter {
	return aiCharacters
}

// getCharacter 根据 ID 查找角色
func getCharacter(id string) *AICharacter {
	for i := range aiCharacters {
		if aiCharacters[i].ID == id {
			return &aiCharacters[i]
		}
	}
	return nil
}

// Chat 与指定 AI 角色对话
// characterID: 角色ID；messages: 历史消息（不含 system prompt，由本函数注入）
func (l *AILogic) Chat(ctx context.Context, characterID string, messages []ChatMessage) (string, error) {
	if l.cfg.APIKey == "" {
		return "", fmt.Errorf("AI 服务未配置 API Key，请设置环境变量 APP_AI_API_KEY")
	}

	character := getCharacter(characterID)
	if character == nil {
		return "", fmt.Errorf("未找到角色: %s", characterID)
	}

	// 组装完整消息：system prompt + 历史消息
	fullMessages := make([]ChatMessage, 0, len(messages)+1)
	fullMessages = append(fullMessages, ChatMessage{
		Role:    "system",
		Content: character.SystemPrompt,
	})
	fullMessages = append(fullMessages, messages...)

	reqBody := chatCompletionRequest{
		Model:    l.cfg.Model,
		Messages: fullMessages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := l.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)

	resp, err := l.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp chatCompletionResponse
		if json.Unmarshal(respBytes, &errResp) == nil && errResp.Error != nil {
			return "", fmt.Errorf("AI 服务错误: %s", errResp.Error.Message)
		}
		return "", fmt.Errorf("AI 服务返回状态码 %d: %s", resp.StatusCode, string(respBytes))
	}

	var result chatCompletionResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("AI 服务返回空响应")
	}

	return result.Choices[0].Message.Content, nil
}

// JsonString 将字符串安全地序列化为 JSON 字符串字面量（含引号与转义）
func JsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ChatStream 与指定 AI 角色流式对话。
// onChunk 每收到一段增量内容即被调用；遇到 [DONE] 或流结束时返回。
func (l *AILogic) ChatStream(ctx context.Context, characterID string, messages []ChatMessage, onChunk func(content string)) error {
	if l.cfg.APIKey == "" {
		return fmt.Errorf("AI 服务未配置 API Key，请设置环境变量 APP_AI_API_KEY")
	}

	character := getCharacter(characterID)
	if character == nil {
		return fmt.Errorf("未找到角色: %s", characterID)
	}

	// 组装完整消息：system prompt + 历史消息
	fullMessages := make([]ChatMessage, 0, len(messages)+1)
	fullMessages = append(fullMessages, ChatMessage{
		Role:    "system",
		Content: character.SystemPrompt,
	})
	fullMessages = append(fullMessages, messages...)

	reqBody := chatCompletionRequest{
		Model:    l.cfg.Model,
		Messages: fullMessages,
		Stream:   true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	url := l.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("调用 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("AI 服务返回状态码 %d: %s", resp.StatusCode, string(respBytes))
	}

	reader := bufio.NewReader(resp.Body)
	// OpenAI 兼容流式 chunk 结构
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data == "" {
					// SSE 心跳/空行，跳过
				} else if data == "[DONE]" {
					return nil
				} else if json.Unmarshal([]byte(data), &chunk) == nil {
					if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
						onChunk(chunk.Choices[0].Delta.Content)
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("读取 AI 流式响应失败: %w", err)
		}
	}
}
