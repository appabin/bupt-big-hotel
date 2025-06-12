package handlers

import (
	"bupt-hotel/database"
	"bupt-hotel/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ACControlRequest 空调控制请求结构
type ACControlRequest struct {
	BillID        int `json:"bill_id"`               // 订单号
	OperationType int    `json:"operation_type"`        // 操作类型：0-开机 1-关机 2-调温
	Speed         string `json:"speed,omitempty"`       // 风速：high/medium/low
	Mode          string `json:"mode,omitempty"`        // 模式：cooling/heating
	TargetTemp    int    `json:"target_temp,omitempty"` // 目标温度*10
}

// ACStatusResponse 空调状态响应结构
type ACStatusResponse struct {
	RoomID          int     `json:"room_id"`
	ACStatus        int     `json:"ac_status"`        // 0: 运行 1: 暂停服务 2: 停机
	Speed           string  `json:"speed"`            // 风速：high/medium/low
	Mode            string  `json:"mode"`             // 模式：cooling/heating
	TargetTemp      int     `json:"target_temp"`      // 目标温度*10
	EnvironmentTemp int     `json:"environment_temp"` // 环境温度*10
	CurrentTemp     int     `json:"current_temp"`     // 当前温度*10
	CurrentCost     float32 `json:"current_cost"`     // 当前花费金额
	TotalCost       float32 `json:"total_cost"`       // 总花费金额
	Rate            float32 `json:"rate"`             // 每分钟费率(元/分钟)
}

// ControlAirConditioner 控制空调
func ControlAirConditioner(c *gin.Context) {
	// 从URL路径参数获取房间ID
	roomID := c.Param("room_id")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间ID不能为空",
		})
		return
	}

	var req ACControlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	// 验证订单号
	if req.BillID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "订单号不能为空",
		})
		return
	}

	// 根据房间ID获取空调信息
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "该房间的空调不存在",
		})
		return
	}

	currentTime := time.Now()

	// 创建空调操作记录
	operation := models.AirConditionerOperation{
		BillID:            req.BillID,
		RoomID:            ac.RoomID,
		AcID:              ac.ID,
		OperationState:    req.OperationType,
		LastOperationTime: currentTime,
	}

	// 根据操作类型设置参数
	switch req.OperationType {
	case 0: // 开机
		operation.Mode = "heating" // 默认制热
		operation.TargetTemp = 220 // 默认22度
		operation.Speed = "medium" // 默认中风
		operation.LastPowerOnTime = currentTime
		operation.SwitchCount = 1
		// 如果用户指定了参数，使用用户参数
		if req.Mode != "" {
			operation.Mode = req.Mode
		}
		if req.TargetTemp > 0 {
			operation.TargetTemp = req.TargetTemp
		}
		if req.Speed != "" {
			operation.Speed = req.Speed
		}

	case 1: // 关机
		// 关机操作，获取当前设置
		var lastOp models.AirConditionerOperation
		if err := database.DB.Where("room_id = ? AND bill_id = ?", ac.RoomID, req.BillID).Order("created_at DESC").First(&lastOp).Error; err == nil {
			operation.Mode = lastOp.Mode
			operation.TargetTemp = lastOp.TargetTemp
			operation.Speed = lastOp.Speed
			operation.SwitchCount = lastOp.SwitchCount + 1
		}

	case 2: // 调温或其他设置
		// 获取当前设置作为基础
		var lastOp models.AirConditionerOperation
		if err := database.DB.Where("room_id = ? AND bill_id = ?", ac.RoomID, req.BillID).Order("created_at DESC").First(&lastOp).Error; err == nil {
			operation.Mode = lastOp.Mode
			operation.TargetTemp = lastOp.TargetTemp
			operation.Speed = lastOp.Speed
			operation.SwitchCount = lastOp.SwitchCount
		}
		// 更新用户指定的设置
		if req.Speed != "" {
			operation.Speed = req.Speed
		}
		if req.Mode != "" {
			operation.Mode = req.Mode
		}
		if req.TargetTemp > 0 {
			operation.TargetTemp = req.TargetTemp
		}
	}

	// 设置环境温度和当前温度
	operation.EnvironmentTemp = ac.EnvironmentTemp
	operation.CurrentTemp = ac.EnvironmentTemp // 初始当前温度等于环境温度

	// 保存操作记录
	if err := database.DB.Create(&operation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "保存操作记录失败",
		})
		return
	}

	// 向调度器发送指令
	scheduler := GetScheduler()
	switch req.OperationType {
	case 0: // 开机
		acRequest := &ACRequest{
			BillID:        req.BillID,
			RoomID:        ac.RoomID,
			ACID:          ac.ID,
			Mode:          operation.Mode,
			CurrentSpeed:  operation.Speed,
			TargetTemp:    operation.TargetTemp,
			RequestTime:   currentTime,
			OperationType: req.OperationType,
		}
		scheduler.AddRequest(acRequest)
	case 1: // 关机
		scheduler.RemoveRequest(ac.RoomID)
	case 2: // 调温或其他设置
		// 先移除旧的请求
		scheduler.RemoveRequest(ac.RoomID)
		// 添加新的请求
		acRequest := &ACRequest{
			BillID:        req.BillID,
			RoomID:        ac.RoomID,
			ACID:          ac.ID,
			Mode:          operation.Mode,
			CurrentSpeed:  operation.Speed,
			TargetTemp:    operation.TargetTemp,
			RequestTime:   currentTime,
			OperationType: req.OperationType,
		}
		scheduler.AddRequest(acRequest)
	}

	// 获取最新的状态记录用于响应
	var detail models.AirConditionerDetail
	if err := database.DB.Where("room_id = ? AND bill_id = ?", ac.RoomID, req.BillID).Order("created_at DESC").First(&detail).Error; err != nil {
		// 如果没有状态记录，返回操作记录信息
		c.JSON(http.StatusOK, gin.H{
			"message": "空调控制成功",
			"data": map[string]interface{}{
				"room_id":        ac.RoomID,
				"bill_id":        req.BillID,
				"operation_type": req.OperationType,
				"message":        "操作成功，等待调度器更新状态",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "空调控制成功",
		"data":    convertToStatusResponse(detail),
	})
}

// GetACStatusLongPolling HTTP长轮询获取空调状态
func GetACStatusLongPolling(c *gin.Context) {
	roomID := c.Param("room_id")
	billIDStr := c.Query("bill_id")
	billID, err := strconv.Atoi(billIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的账单ID",
		})
		return
	}
	
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间ID不能为空",
		})
		return
	}
	
	if billID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "订单号不能为空",
		})
		return
	}

	// 获取初始状态
	initialStatus := getACCurrentStatus(roomID, billID)
	if initialStatus == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "空调状态不存在",
		})
		return
	}

	// 长轮询逻辑：每1秒检查一次状态变化，最多等待10秒
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// 超时，返回当前状态
			currentStatus := getACCurrentStatus(roomID, billID)
			c.JSON(http.StatusOK, gin.H{
				"message": "获取空调状态成功",
				"data":    currentStatus,
			})
			return

		case <-ticker.C:
			// 检查状态是否有变化
			currentStatus := getACCurrentStatus(roomID, billID)
			if currentStatus != nil && hasStatusChanged(*initialStatus, *currentStatus) {
				c.JSON(http.StatusOK, gin.H{
					"message": "获取空调状态成功",
					"data":    currentStatus,
				})
				return
			}

		case <-c.Request.Context().Done():
			// 客户端断开连接
			return
		}
	}
}

// getACCurrentStatus 获取空调当前状态
func getACCurrentStatus(roomID string, billID int) *ACStatusResponse {
	// 获取该房间和订单的最新空调状态记录
	var detail models.AirConditionerDetail
	if err := database.DB.Where("room_id = ? AND bill_id = ?", roomID, billID).Order("created_at DESC").First(&detail).Error; err != nil {
		// 如果没有状态记录，尝试从操作记录获取基础信息
		var operation models.AirConditionerOperation
		if err := database.DB.Where("room_id = ? AND bill_id = ?", roomID, billID).Order("created_at DESC").First(&operation).Error; err != nil {
			return nil
		}
		
		// 根据操作记录构造状态响应
		roomIDInt, _ := strconv.Atoi(roomID)
		return &ACStatusResponse{
			RoomID:          roomIDInt,
			ACStatus:        operation.OperationState, // 使用操作状态
			Speed:           operation.Speed,
			Mode:            operation.Mode,
			TargetTemp:      operation.TargetTemp,
			EnvironmentTemp: operation.EnvironmentTemp,
			CurrentTemp:     operation.CurrentTemp,
			CurrentCost:     operation.CurrentCost,
			TotalCost:       operation.TotalCost,
			Rate:            1.0,
		}
	}

	return convertToStatusResponse(detail)
}

// convertToStatusResponse 转换为状态响应格式
func convertToStatusResponse(detail models.AirConditionerDetail) *ACStatusResponse {
	return &ACStatusResponse{
		RoomID:          detail.RoomID,
		ACStatus:        detail.ACStatus,
		Speed:           detail.Speed,
		Mode:            detail.Mode,
		TargetTemp:      detail.TargetTemp,
		EnvironmentTemp: detail.EnvironmentTemp,
		CurrentTemp:     detail.CurrentTemp,
		CurrentCost:     detail.CurrentCost,
		TotalCost:       detail.TotalCost,
		Rate:            detail.Rate,
	}
}

// hasStatusChanged 检查状态是否发生变化
func hasStatusChanged(oldStatus, newStatus ACStatusResponse) bool {
	return oldStatus.ACStatus != newStatus.ACStatus ||
		oldStatus.CurrentTemp != newStatus.CurrentTemp ||
		oldStatus.CurrentCost != newStatus.CurrentCost ||
		oldStatus.TotalCost != newStatus.TotalCost ||
		oldStatus.Speed != newStatus.Speed ||
		oldStatus.Mode != newStatus.Mode ||
		oldStatus.TargetTemp != newStatus.TargetTemp
}
