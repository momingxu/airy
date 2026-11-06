package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"airy/internal/database"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	sessionCookieName = "airy_session"
	sessionLifetime   = 30 * 24 * time.Hour
	authUserKey       = "authenticated_user"
	authSessionKey    = "authenticated_session"
)

type authCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func authStatusHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var user database.User
		err := db.WithContext(c.Request.Context()).Order("id ASC").First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"configured": false, "authenticated": false})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取登录状态失败"})
			return
		}
		_, authenticated := authenticatedSession(c, db)
		response := gin.H{"configured": true, "authenticated": authenticated}
		if authenticated {
			response["username"] = user.Username
		}
		c.JSON(http.StatusOK, response)
	}
}

func setupHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body authCredentials
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名和密码"})
			return
		}
		username, err := validateCredentials(body.Username, body.Password)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建密码失败"})
			return
		}

		user := database.User{ID: 1, Username: username, PasswordHash: string(passwordHash)}
		if err := db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&database.User{}).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return errAccountConfigured
			}
			return tx.Create(&user).Error
		}); err != nil {
			if errors.Is(err, errAccountConfigured) || isUniqueViolation(err) {
				c.JSON(http.StatusConflict, gin.H{"error": "管理员已经创建，请直接登录"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建管理员失败"})
			return
		}
		if err := issueSession(c, db, user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建登录会话失败"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"user": gin.H{"username": user.Username}})
	}
}

var errAccountConfigured = errors.New("account already configured")

func loginHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body authCredentials
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入用户名和密码"})
			return
		}
		var user database.User
		if err := db.WithContext(c.Request.Context()).Order("id ASC").First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusPreconditionRequired, gin.H{"error": "请先创建管理员", "setup_required": true})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
			return
		}
		if !strings.EqualFold(strings.TrimSpace(body.Username), user.Username) ||
			bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
			return
		}
		if err := issueSession(c, db, user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建登录会话失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": gin.H{"username": user.Username}})
	}
}

func logoutHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token, err := c.Cookie(sessionCookieName); err == nil && token != "" {
			_ = db.WithContext(c.Request.Context()).Where("token_hash = ?", hashSessionToken(token)).Delete(&database.UserSession{}).Error
		}
		clearSessionCookie(c)
		c.Status(http.StatusNoContent)
	}
}

func updateAccountHandler(db *gorm.DB) gin.HandlerFunc {
	type requestBody struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	return func(c *gin.Context) {
		user, ok := currentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		var body requestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "设置内容无效"})
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.CurrentPassword)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "当前密码错误"})
			return
		}
		username := strings.TrimSpace(body.Username)
		if username == "" {
			username = user.Username
		}
		if len([]rune(username)) > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能超过 64 个字符"})
			return
		}
		updates := map[string]any{"username": username}
		if body.NewPassword != "" {
			if err := validatePassword(body.NewPassword); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "更新密码失败"})
				return
			}
			updates["password_hash"] = string(hash)
		}
		if username == user.Username && body.NewPassword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "没有需要保存的更改"})
			return
		}

		token, tokenHash, expiresAt, err := newSessionToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "更新账户失败"})
			return
		}
		err = db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&database.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ?", user.ID).Delete(&database.UserSession{}).Error; err != nil {
				return err
			}
			return tx.Create(&database.UserSession{UserID: user.ID, TokenHash: tokenHash, ExpiresAt: expiresAt}).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存账户设置失败"})
			return
		}
		setSessionCookie(c, token, expiresAt)
		c.JSON(http.StatusOK, gin.H{"user": gin.H{"username": username}})
	}
}

func requireAuthentication(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := authenticatedSession(c, db)
		if !ok {
			clearSessionCookie(c)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			return
		}
		c.Set(authUserKey, session.User)
		c.Set(authSessionKey, session)
		c.Next()
	}
}

func authenticatedSession(c *gin.Context, db *gorm.DB) (database.UserSession, bool) {
	token, err := c.Cookie(sessionCookieName)
	if err != nil || token == "" {
		return database.UserSession{}, false
	}
	now := time.Now().UTC()
	var session database.UserSession
	err = db.WithContext(c.Request.Context()).Preload("User").
		Where("token_hash = ? AND expires_at > ?", hashSessionToken(token), now).
		First(&session).Error
	return session, err == nil && session.User.ID != 0
}

func currentUser(c *gin.Context) (database.User, bool) {
	value, exists := c.Get(authUserKey)
	if !exists {
		return database.User{}, false
	}
	user, ok := value.(database.User)
	return user, ok
}

func validateCredentials(rawUsername, password string) (string, error) {
	username := strings.TrimSpace(rawUsername)
	if username == "" {
		return "", errors.New("用户名不能为空")
	}
	if len([]rune(username)) > 64 {
		return "", errors.New("用户名不能超过 64 个字符")
	}
	if err := validatePassword(password); err != nil {
		return "", err
	}
	return username, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("密码至少需要 8 个字符")
	}
	if len(password) > 72 {
		return errors.New("密码不能超过 72 个字符")
	}
	return nil
}

func issueSession(c *gin.Context, db *gorm.DB, userID uint) error {
	token, tokenHash, expiresAt, err := newSessionToken()
	if err != nil {
		return err
	}
	_ = db.WithContext(c.Request.Context()).Where("expires_at <= ?", time.Now().UTC()).Delete(&database.UserSession{}).Error
	if err := db.WithContext(c.Request.Context()).Create(&database.UserSession{
		UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt,
	}).Error; err != nil {
		return err
	}
	setSessionCookie(c, token, expiresAt)
	return nil
}

func newSessionToken() (string, string, time.Time, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	return token, hashSessionToken(token), time.Now().UTC().Add(sessionLifetime), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secureRequest(c.Request), SameSite: http.SameSiteLaxMode,
		Expires: expiresAt, MaxAge: int(time.Until(expiresAt).Seconds()),
	})
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: secureRequest(c.Request), SameSite: http.SameSiteLaxMode,
		Expires: time.Unix(0, 0), MaxAge: -1,
	})
}

func secureRequest(request *http.Request) bool {
	return request.TLS != nil || strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")), "https")
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
