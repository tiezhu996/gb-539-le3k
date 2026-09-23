package handler

import (
	"github.com/gin-gonic/gin"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type AuthHandler struct{ Service service.AuthService }

func (h AuthHandler) Login(c *gin.Context) {
	var input dto.LoginRequest
	if !bind(c, &input) {
		return
	}
	token, user, err := h.Service.Login(c, input.Email, input.Password)
	if err != nil {
		c.JSON(401, gin.H{"error": gin.H{"code": "invalid_credentials", "message": "邮箱或密码错误"}})
		return
	}
	util.OK(c, dto.LoginResponse{Token: token, User: user})
}
func (h AuthHandler) Me(c *gin.Context) {
	util.OK(c, gin.H{"id": actor(c), "email": c.GetString("user_email"), "role": c.GetString("role")})
}
