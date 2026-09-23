package handler

import (
	"github.com/gin-gonic/gin"
	"timber-kiln-drying-optimizer/backend/internal/dto"
	"timber-kiln-drying-optimizer/backend/internal/service"
	"timber-kiln-drying-optimizer/backend/internal/util"
)

type ScheduleHandler struct{ Service service.ScheduleService }

func (h ScheduleHandler) List(c *gin.Context) {
	items, err := h.Service.List(c)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, items)
}
func (h ScheduleHandler) Get(c *gin.Context) {
	item, err := h.Service.Get(c, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}
func (h ScheduleHandler) Calculate(c *gin.Context) {
	var input dto.ScheduleCalculate
	if !bind(c, &input) {
		return
	}
	input.IdempotencyKey = c.GetHeader("Idempotency-Key")
	item, err := h.Service.Calculate(c, input, actor(c), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	util.Created(c, item)
}
func (h ScheduleHandler) Review(c *gin.Context) {
	var input dto.ScheduleReview
	if !bind(c, &input) {
		return
	}
	item, err := h.Service.Review(c, c.Param("id"), input.Decision, input.Note, actor(c), requestID(c), input.Version)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}

func (h ScheduleHandler) Freeze(c *gin.Context) {
	var input dto.ScheduleFreeze
	if !bind(c, &input) {
		return
	}
	item, err := h.Service.Freeze(c, c.Param("id"), actor(c), requestID(c), input.Version)
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, item)
}

func (h ScheduleHandler) Compare(c *gin.Context) {
	var input dto.ScheduleCompare
	if !bind(c, &input) {
		return
	}
	comparison, err := h.Service.Compare(c, c.Param("id"), input.BaselineScheduleID, actor(c), requestID(c))
	if err != nil {
		fail(c, err)
		return
	}
	util.OK(c, comparison)
}
