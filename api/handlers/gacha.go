package handlers

import (
	"net/http"

	"girls-rating-api/pkg/response"

	"github.com/gin-gonic/gin"
)

// maxDeviceIDLength 与 gacha_draw.device_id VARCHAR(64) 对齐
const maxDeviceIDLength = 64

// requireDeviceID 抽卡三接口的公共前置：X-Device-Id 必填（匿名设备标识）。
func requireDeviceID(c *gin.Context) (string, bool) {
	deviceID := c.GetHeader("X-Device-Id")
	if deviceID == "" {
		response.Error(c, http.StatusBadRequest, "missing X-Device-Id header")
		return "", false
	}
	if len(deviceID) > maxDeviceIDLength {
		response.Error(c, http.StatusBadRequest, "invalid X-Device-Id header")
		return "", false
	}
	return deviceID, true
}

// GachaDraw 抽卡（幂等补发）
// @Summary      每日抽卡（幂等补发）
// @Description  每台设备每天一次、固定 10 张；当天已抽时补发当日快照（replayed=true），不返回 409。请求头携带 X-Device-Id
// @Tags         抽卡
// @Produce      json
// @Param        X-Device-Id header string true "设备标识（前端 crypto.randomUUID() 生成并持久化）"
// @Success      200  {object}  response.APIResponse  "data: { date, replayed, cards, stats }"
// @Failure      400  {object}  response.APIResponse  "缺少 X-Device-Id"
// @Failure      500  {object}  response.APIResponse  "服务器错误（池空/存储失败）"
// @Router       /api/gacha/draw [post]
func (s *Server) GachaDraw(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}

	result, err := s.GachaService.Draw(c.Request.Context(), deviceID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusOK, result)
}

// GachaStatus 今日状态 + 公示概率
// @Summary      抽卡状态（今日能否抽 + 公示概率 + 累计统计）
// @Description  前端开屏调用：drawnToday=false 且当日未弹过 → 每日弹窗；drawnToday=true → 抽卡页直接进展示态
// @Tags         抽卡
// @Produce      json
// @Param        X-Device-Id header string true "设备标识"
// @Success      200  {object}  response.APIResponse  "data: { date, drawnToday, cards, probabilities, stats, pool }"
// @Failure      400  {object}  response.APIResponse  "缺少 X-Device-Id"
// @Failure      500  {object}  response.APIResponse  "服务器错误"
// @Router       /api/gacha/status [get]
func (s *Server) GachaStatus(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}

	result, err := s.GachaService.Status(c.Request.Context(), deviceID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusOK, result)
}

// GachaCollection 卡册分页
// @Summary      卡册分页（按设备）
// @Description  全部快照按时间倒序展开，应用层切片分页；每张卡带 drawDate
// @Tags         抽卡
// @Produce      json
// @Param        X-Device-Id header string true "设备标识"
// @Param        page     query     int  false  "页码，从 1 开始"  default(1)
// @Param        pageSize query     int  false  "每页数量"  default(30)
// @Success      200  {object}  response.APIResponse  "data: { list, total, page, pageSize, stats }"
// @Failure      400  {object}  response.APIResponse  "缺少 X-Device-Id"
// @Failure      500  {object}  response.APIResponse  "服务器错误"
// @Router       /api/gacha/collection [get]
func (s *Server) GachaCollection(c *gin.Context) {
	deviceID, ok := requireDeviceID(c)
	if !ok {
		return
	}

	page, pageSize := parsePagination(c, 30)

	result, err := s.GachaService.Collection(c.Request.Context(), deviceID, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusOK, result)
}
