package handler

import (
	"github.com/gin-gonic/gin"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type ReadingHandler struct{ Service service.ReadingService }

func (h ReadingHandler) List(c *gin.Context) {
	items, err := h.Service.List(c, c.Query("lot_id"))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, items)
}
func (h ReadingHandler) Import(c *gin.Context) {
	var input dto.ReadingImport
	if !bind(c, &input) {
		return
	}
	items, err := h.Service.Import(c, input, actor(c), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	util.Created(c, items)
}

func (h ReadingHandler) Void(c *gin.Context) {
	var input dto.ReadingVoid
	if !bind(c, &input) {
		return
	}
	item, err := h.Service.Void(c, c.Param("id"), input, actor(c), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}

func (h ReadingHandler) Correct(c *gin.Context) {
	var input struct {
		dto.ReadingImport
		Version int `json:"version" binding:"required,gt=0"`
	}
	if !bind(c, &input) {
		return
	}
	items, err := h.Service.Correct(c, c.Param("id"), input.ReadingImport, actor(c), requestID(c), input.Version)
	if err != nil {
		fail(c, err)
		return
	}
	util.Created(c, items)
}
