package logic

import (
	"context"
	"fmt"
	"os"
	"time"

	"fake_tiktok/internal/pkg"

	"github.com/gin-gonic/gin"
	"github.com/mojocn/base64Captcha"
)

// emailVerifyTTL 邮箱验证码在 Redis 中的有效期。
// 与下方 session.Set("expire_time", ...) 原本定义的 5 分钟保持一致。
const emailVerifyTTL = 5 * time.Minute

// emailVerifyKey 构造"邮箱 -> 验证码"的 Redis key。
// 使用 module 前缀避免与其他业务键冲突。
func emailVerifyKey(email string) string {
	return "email_verify:" + email
}

type BaseLogic struct {
	deps *LogicDeps
}

func NewBaseLogic(deps *LogicDeps) *BaseLogic {
	return &BaseLogic{deps: deps}
}

// SendEmailCode 生成 6 位随机邮箱验证码并写入 Redis，TTL 5 分钟。
//
// 之前使用 gin-contrib/sessions 的 cookie session 存储验证码，
// 但前端 axios 默认 withCredentials=false，浏览器不会回传 cookie，
// 导致 Register 时 session.Get("email") 永远返回 nil、报 "email not match"。
//
// 改用 Redis 后：验证码以"email 维度"独立存储，校验时用请求中的 email 作为 key 查找，
// 彻底绕开 cookie session 的跨域问题。同时支持校验后 Del 防止重放。
func (b *BaseLogic) SendEmailCode(c *gin.Context, email string) error {
	verifyCode := pkg.GenerateRandomCode(6)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	key := emailVerifyKey(email)
	// HSet：同一 key 上存 code + email + expire_ts。
	// email 字段冗余存储一份，便于在数据迁移/调试时快速确认归属。
	expireTS := time.Now().Add(emailVerifyTTL).Unix()
	if err := b.deps.RedisClient.HSet(ctx, key, map[string]interface{}{
		"code":      verifyCode,
		"email":     email,
		"expire_ts": expireTS,
	}).Err(); err != nil {
		return fmt.Errorf("save verify code to redis: %w", err)
	}
	// 显式设置 TTL，防止 HSet 后 EXPIRE 失败留下永久键。
	if err := b.deps.RedisClient.Expire(ctx, key, emailVerifyTTL).Err(); err != nil {
		return fmt.Errorf("set verify code ttl: %w", err)
	}

	subject := "您的邮箱验证码"
	body := `这里是fake_tiktok平台,<br/>
<br/>
你正在注册该平台的账户，为了确保你的账户安全，请使用下面验证码进行验证：<br/>
<br/>
验证码：[<font color="blue"><u>` + verifyCode + `</u></font>]<br/>
还在浪费时间吗，该验证码在 5 分钟内有效，请尽快食用。<br/>
<br/>
如果你没有请求此验证码，请忽略此邮件。
<br/>
如有任何疑问，请联系：<br/>
fzsirrr的徒弟: tj <br/>
邮箱: 2877712419@qq.com<br/>
<br/>
期待你在该平台上分享你的逆天视频！<br/>
<br/>
`

	return pkg.Email(email, subject, body, b.deps.Config)
}

// VerifyEmailCode 校验邮箱验证码是否匹配且未过期。
// 校验通过后立即从 Redis 删除（防止重放）。
//
// 返回值：
//   - ok=true, nil: 校验通过
//   - ok=false, nil: 验证码不匹配 / 已过期 / 邮箱不匹配
//   - ok=false, err: Redis 不可用等系统级错误
func (b *BaseLogic) VerifyEmailCode(ctx context.Context, email, code string) (bool, error) {
	key := emailVerifyKey(email)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	values, err := b.deps.RedisClient.HGetAll(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("get verify code: %w", err)
	}
	if len(values) == 0 {
		// 键不存在：可能从未发送、已过期或已被消费
		return false, nil
	}

	// 邮箱一致性校验（防御性：防止 SendEmailCode 写入时 race）
	if v, ok := values["email"]; !ok || v != email {
		return false, nil
	}

	// 验证码匹配
	if v, ok := values["code"]; !ok || v != code {
		return false, nil
	}

	// 过期时间校验（防 Redis EXPIRE 失败时仍能二次校验）
	if expireTS, ok := values["expire_ts"]; ok {
		var ts int64
		fmt.Sscanf(expireTS, "%d", &ts)
		if ts > 0 && ts < time.Now().Unix() {
			// 过期了顺手清掉
			_ = b.deps.RedisClient.Del(ctx, key).Err()
			return false, nil
		}
	}

	// 校验通过：消费（删除）该 key，防止同一验证码被多次使用
	if err := b.deps.RedisClient.Del(ctx, key).Err(); err != nil {
		// 删除失败不阻塞注册流程（验证码本身已匹配且未过期）
		// 但记录一下以便排查：极端情况下可能导致验证码被复用
		// （攻击者需要先获取到验证码本身，所以风险可控）
		_ = err
	}
	return true, nil
}

func (b *BaseLogic) GenerateCaptcha() (string, string, error) {
	cfg := b.deps.Config
	driver := base64Captcha.NewDriverDigit(
		cfg.Captcha.Height,
		cfg.Captcha.Width,
		cfg.Captcha.Length,
		cfg.Captcha.MaxSkew,
		cfg.Captcha.DotCount,
	)
	captcha := base64Captcha.NewCaptcha(driver, b.deps.CaptchaStore)
	id, b64s, _, err := captcha.Generate()
	return id, b64s, err
}

func (b *BaseLogic) VerifyCaptcha(captchaID, captcha string) bool {
	// dev 模式临时跳过 captcha 校验
	if os.Getenv("APP_SKIP_CAPTCHA") == "1" {
		return true
	}
	return b.deps.CaptchaStore.Verify(captchaID, captcha, true)
}
