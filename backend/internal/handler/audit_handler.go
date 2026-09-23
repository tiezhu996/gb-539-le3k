package handler

import (
	"github.com/gin-gonic/gin"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type AuditHandler struct{ Service service.AuditService }

func (h AuditHandler) List(c *gin.Context) {
	items, err := h.Service.List(c)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, items)
}
