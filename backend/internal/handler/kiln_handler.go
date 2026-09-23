package handler

import (
	"github.com/gin-gonic/gin"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type KilnHandler struct{ Service service.KilnService }

func (h KilnHandler) List(c *gin.Context) {
	items, err := h.Service.List(c)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, items)
}
func (h KilnHandler) Get(c *gin.Context) {
	item, err := h.Service.Get(c, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}
func (h KilnHandler) Create(c *gin.Context) {
	var input dto.KilnCreate
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
func (h KilnHandler) Update(c *gin.Context) {
	var input dto.KilnUpdate
	if !bind(c, &input) {
		return
	}
	item, err := h.Service.Update(c, c.Param("id"), input, actor(c), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}
