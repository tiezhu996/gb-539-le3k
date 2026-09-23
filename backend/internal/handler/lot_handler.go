package handler

import (
	"github.com/gin-gonic/gin"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type LotHandler struct{ Service service.LotService }

func (h LotHandler) List(c *gin.Context) {
	items, err := h.Service.List(c)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, items)
}
func (h LotHandler) Get(c *gin.Context) {
	item, err := h.Service.Get(c, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}
func (h LotHandler) Create(c *gin.Context) {
	var input dto.LotCreate
	if !bind(c, &input) {
		return
	}
	item, err := h.Service.Create(c, input, actor(c), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	util.Created(c, item)
}
func (h LotHandler) Transition(c *gin.Context) {
	var input struct {
		State   string `json:"state" binding:"required"`
		Version int    `json:"version" binding:"required,gt=0"`
	}
	if !bind(c, &input) {
		return
	}
	item, err := h.Service.Transition(c, c.Param("id"), input.State, actor(c), requestID(c), input.Version)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}
