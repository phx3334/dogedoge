package logic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fake_tiktok/internal/dto/response"

	"go.uber.org/zap"
)

type GaodeLogic struct {
	APIKey string
	Client *http.Client
	Logger *zap.Logger
}

func NewGaodeLogic(apiKey string, logger *zap.Logger) *GaodeLogic {
	return &GaodeLogic{
		APIKey: apiKey,
		Client: &http.Client{
			Timeout: 5 * time.Second,
		},
		Logger: logger,
	}
}

func (l *GaodeLogic) GetLocationByIP(ip string) (*response.GaodeIPResponse, error) {
	url := fmt.Sprintf("https://restapi.amap.com/v3/ip?ip=%s&key=%s", ip, l.APIKey)

	resp, err := l.Client.Get(url)
	if err != nil {
		l.Logger.Error("请求高德地图API失败", zap.Error(err), zap.String("ip", ip))
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		l.Logger.Error("读取响应失败", zap.Error(err))
		return nil, err
	}

	var result response.GaodeIPResponse
	if err := json.Unmarshal(body, &result); err != nil {
		l.Logger.Error("解析响应失败", zap.Error(err), zap.String("body", string(body)))
		return nil, err
	}

	if result.Status != "1" {
		l.Logger.Error("高德地图API返回错误", zap.String("info", result.Info))
		return nil, fmt.Errorf("高德地图API错误: %s", result.Info)
	}

	return &result, nil
}

func (l *GaodeLogic) GetAddressByIP(ip string) string {
	result, err := l.GetLocationByIP(ip)
	if err != nil || result.Province == "" {
		return "未知"
	}

	if result.City != "" && result.Province != result.City {
		return result.Province + "-" + result.City
	}
	return result.Province
}
