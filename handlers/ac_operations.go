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
	ACState      *int    `json:"ac_state,omitempty"`      // 0: 关闭 1: 开启
	Mode         *string `json:"mode,omitempty"`          // cooling/heating
	CurrentSpeed *string `json:"current_speed,omitempty"` // low/medium/high
	TargetTemp   *int    `json:"target_temp,omitempty"`   // 目标温度*10，如245表示24.5度
}

// ACStatusResponse 空调状态响应结构
type ACStatusResponse struct {
	ServiceStatus     string  `json:"service_status"`      // 服务状态：服务中/等待服务/暂停服务/已关机
	CurrentTemp       int     `json:"current_temp"`        // 当前温度*10，如245表示24.5度
	TargetTemp        int     `json:"target_temp"`         // 目标温度*10，如245表示24.5度
	CurrentSpeed      string  `json:"current_speed"`       // 当前风速
	TotalCost         float32 `json:"total_cost"`          // 累计总费用
	SessionCost       float32 `json:"session_cost"`        // 本次开机费用
	ACState           int     `json:"ac_state"`            // 空调状态
	Mode              string  `json:"mode"`                // 模式
	QueuePosition     int     `json:"queue_position"`      // 队列位置（0表示不在队列中）
	EstimatedWaitTime int     `json:"estimated_wait_time"` // 预计等待时间（分钟）
}

// GetAirConditioner 获取房间空调信息
func GetAirConditioner(c *gin.Context) {
	roomIDStr := c.Param("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的房间ID",
		})
		return
	}

	// 检查用户是否有权限访问该房间
	if !checkRoomPermission(c, roomID) {
		return
	}

	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "空调信息不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "获取空调信息成功",
		"air_conditioner": ac,
	})
}

// ControlAirConditioner 控制空调
func ControlAirConditioner(c *gin.Context) {
	roomIDStr := c.Param("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的房间ID",
		})
		return
	}

	// 检查用户是否有权限访问该房间
	if !checkRoomPermission(c, roomID) {
		return
	}

	var req ACControlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	// 获取调度器
	scheduler := GetScheduler()

	// 更新空调状态
	updated := false
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "空调信息不存在",
		})
		return
	}

	if req.ACState != nil {
		if ac.ACState != *req.ACState {
			ac.ACState = *req.ACState
			if ac.ACState == 1 {
				ac.LastPowerOnTime = time.Now()
			} else {
				// 关机时从调度器中移除
				scheduler.RemoveRequest(roomID)
			}
			updated = true
		}
	}

	if req.Mode != nil && ac.Mode != *req.Mode {
		ac.Mode = *req.Mode
		updated = true
	}

	if req.CurrentSpeed != nil && ac.CurrentSpeed != *req.CurrentSpeed {
		ac.CurrentSpeed = *req.CurrentSpeed
		updated = true
	}

	if req.TargetTemp != nil && ac.TargetTemp != *req.TargetTemp {
		ac.TargetTemp = *req.TargetTemp
		updated = true
	}

	if updated {
		if err := database.DB.Save(&ac).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "更新空调状态失败",
			})
			return
		}
	}

	// 如果空调开启且有温控需求，添加到调度器
	if ac.ACState == 1 && ac.TargetTemp != ac.CurrentTemp {
		acRequest := &ACRequest{
			RoomID:       roomID,
			Mode:         ac.Mode,
			CurrentSpeed: ac.CurrentSpeed,
			TargetTemp:   ac.TargetTemp,
			RequestTime:  time.Now(),
		}
		scheduler.AddRequest(acRequest)
	}
	c.JSON(http.StatusOK, gin.H{
		"message":         "空调控制成功",
		"air_conditioner": ac,
	})
}

// GetACStatus 获取空调状态
func GetACStatus(c *gin.Context) {
	roomIDStr := c.Param("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的房间ID",
		})
		return
	}

	// 检查用户是否有权限访问该房间
	if !checkRoomPermission(c, roomID) {
		return
	}

	// 获取空调状态
	status := getACStatus(roomID)

	c.JSON(http.StatusOK, gin.H{
		"message": "获取空调状态成功",
		"status":  status,
	})
}

// GetACStatusLongPolling 长轮询获取空调状态
func GetACStatusLongPolling(c *gin.Context) {
	roomIDStr := c.Param("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的房间ID",
		})
		return
	}

	// 检查用户是否有权限访问该房间
	if !checkRoomPermission(c, roomID) {
		return
	}

	// 获取初始状态
	initialStatus := getACStatus(roomID)

	// 长轮询逻辑：每5秒检查一次状态变化，最多等待30秒
	timeout := time.After(3 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// 超时，返回当前状态
			c.JSON(http.StatusOK, gin.H{
				"message": "获取空调状态成功",
				"status":  getACStatus(roomID),
			})
			return
		case <-ticker.C:
			// 检查状态是否有变化
			currentStatus := getACStatus(roomID)
			if hasStatusChanged(initialStatus, currentStatus) {
				c.JSON(http.StatusOK, gin.H{
					"message": "获取空调状态成功",
					"status":  currentStatus,
				})
				return
			}
		case <-c.Request.Context().Done():
			// 客户端断开连接
			return
		}
	}
}

// GetSchedulerStatus 获取调度器状态（管理员权限）
func GetSchedulerStatus(c *gin.Context) {
	// 这里应该检查管理员权限，暂时省略

	scheduler := GetScheduler()
	servingQueue := scheduler.GetServingQueue()
	waitingQueue := scheduler.GetWaitingQueue()

	c.JSON(http.StatusOK, gin.H{
		"message":       "获取调度器状态成功",
		"serving_queue": servingQueue,
		"waiting_queue": waitingQueue,
	})
}

// GetAllAirConditioners 获取所有空调信息（管理员权限）
func GetAllAirConditioners(c *gin.Context) {
	var acs []models.AirConditioner
	if err := database.DB.Find(&acs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取空调信息失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "获取所有空调信息成功",
		"air_conditioners": acs,
	})
}

// checkRoomPermission 检查用户是否有权限访问指定房间
func checkRoomPermission(c *gin.Context, roomID int) bool {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "用户未登录",
		})
		return false
	}

	// 检查用户是否有权限访问该房间
	var room models.RoomInfo
	if err := database.DB.Where("room_id = ? AND client_id = ? AND state = ?", roomID, strconv.Itoa(userID.(int)), 1).First(&room).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "您没有权限访问此房间或房间未入住",
		})
		return false
	}

	return true
}

// getACStatus 获取空调详细状态
func getACStatus(roomID int) ACStatusResponse {
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err != nil {
		return ACStatusResponse{}
	}

	// 获取调度器状态
	scheduler := GetScheduler()
	servingQueue := scheduler.GetServingQueue()
	waitingQueue := scheduler.GetWaitingQueue()

	// 检查服务状态
	serviceStatus := "暂停服务"
	queuePosition := 0
	estimatedWaitTime := 0

	if ac.ACState == 1 {
		// 检查是否在服务队列中
		for _, req := range servingQueue {
			if req.RoomID == roomID {
				serviceStatus = "服务中"
				break
			}
		}

		// 如果不在服务队列，检查等待队列
		if serviceStatus == "暂停服务" {
			for i, req := range waitingQueue {
				if req.RoomID == roomID {
					serviceStatus = "等待服务"
					queuePosition = i + 1
					// 估算等待时间：前面每个请求2分钟 + 当前服务队列剩余时间
					estimatedWaitTime = (i + 1) * 2
					break
				}
			}
		}
	} else {
		serviceStatus = "已关机"
	}

	// 计算费用
	totalCost := calculateTotalCost(roomID)
	sessionCost := calculateSessionCost(roomID, ac.LastPowerOnTime)

	return ACStatusResponse{
		ServiceStatus:     serviceStatus,
		CurrentTemp:       ac.CurrentTemp, // 直接返回存储的整数温度值
		TargetTemp:        ac.TargetTemp,  // 直接返回存储的整数温度值
		CurrentSpeed:      ac.CurrentSpeed,
		TotalCost:         totalCost,
		SessionCost:       sessionCost,
		ACState:           ac.ACState,
		Mode:              ac.Mode,
		QueuePosition:     queuePosition,
		EstimatedWaitTime: estimatedWaitTime,
	}
}

// hasStatusChanged 检查状态是否发生变化
func hasStatusChanged(oldStatus, newStatus ACStatusResponse) bool {
	return oldStatus.ServiceStatus != newStatus.ServiceStatus ||
		oldStatus.CurrentTemp != newStatus.CurrentTemp ||
		oldStatus.QueuePosition != newStatus.QueuePosition ||
		oldStatus.EstimatedWaitTime != newStatus.EstimatedWaitTime ||
		oldStatus.ACState != newStatus.ACState
}

// calculateTotalCost 计算累计总费用
func calculateTotalCost(roomID int) float32 {
	var totalCost float32
	database.DB.Model(&models.Detail{}).Where("room_id = ?", roomID).Select("COALESCE(SUM(cost), 0)").Scan(&totalCost)
	return totalCost
}

// calculateSessionCost 计算本次开机费用
func calculateSessionCost(roomID int, lastPowerOnTime time.Time) float32 {
	if lastPowerOnTime.IsZero() {
		return 0
	}

	var sessionCost float32
	database.DB.Model(&models.Detail{}).Where("room_id = ? AND start_time >= ?", roomID, lastPowerOnTime).Select("COALESCE(SUM(cost), 0)").Scan(&sessionCost)
	return sessionCost
}
