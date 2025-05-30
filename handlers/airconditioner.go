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
	ACState      *int     `json:"ac_state,omitempty"`      // 0: 关闭 1: 开启
	Mode         *string  `json:"mode,omitempty"`          // cooling/heating
	CurrentSpeed *string  `json:"current_speed,omitempty"` // low/medium/high
	TargetTemp   *float32 `json:"target_temp,omitempty"`   // 目标温度
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

	// 检查用户是否有权限控制该房间空调
	if !checkRoomPermission(c, roomID) {
		return
	}

	var req ACControlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取空调信息
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "空调信息不存在",
		})
		return
	}

	// 更新空调状态
	updated := false

	if req.ACState != nil {
		if *req.ACState != ac.ACState {
			ac.ACState = *req.ACState
			if *req.ACState == 1 {
				// 开机时记录时间并增加开关次数
				ac.LastPowerOnTime = time.Now()
				ac.SwitchCount++
			}
			updated = true
		}
	}

	if req.Mode != nil && (*req.Mode == "cooling" || *req.Mode == "heating") {
		ac.Mode = *req.Mode
		updated = true
	}

	if req.CurrentSpeed != nil && (*req.CurrentSpeed == "low" || *req.CurrentSpeed == "medium" || *req.CurrentSpeed == "high") {
		ac.CurrentSpeed = *req.CurrentSpeed
		updated = true
	}

	if req.TargetTemp != nil {
		// 温度范围限制
		if *req.TargetTemp >= 16.0 && *req.TargetTemp <= 30.0 {
			ac.TargetTemp = *req.TargetTemp
			updated = true
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "目标温度必须在16-30度之间",
			})
			return
		}
	}

	if !updated {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "没有有效的更新参数",
		})
		return
	}

	// 模拟温度变化（简单的温度调节逻辑）
	if ac.ACState == 1 {
		updateCurrentTemperature(&ac)
	}

	if err := database.DB.Save(&ac).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "空调控制失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "空调控制成功",
		"air_conditioner": ac,
	})
}

// checkRoomPermission 检查用户是否有权限访问房间
func checkRoomPermission(c *gin.Context, roomID int) bool {
	userID, _ := c.Get("user_id")
	identity, _ := c.Get("identity")

	// 管理员有所有权限
	if identity == "administrator" {
		return true
	}

	// 普通用户只能控制自己入住的房间
	var room models.RoomInfo
	if err := database.DB.Where("room_id = ? AND client_id = ? AND state = ?", roomID, strconv.Itoa(userID.(int)), 1).First(&room).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "您没有权限访问此房间或房间未入住",
		})
		return false
	}

	return true
}

// updateCurrentTemperature 更新当前温度（简单的温度调节模拟）
func updateCurrentTemperature(ac *models.AirConditioner) {
	if ac.ACState == 0 {
		return
	}

	// 简单的温度调节逻辑
	tempDiff := ac.TargetTemp - ac.CurrentTemp
	var speedFactor float32 = 0.1 // 默认低速

	switch ac.CurrentSpeed {
	case "medium":
		speedFactor = 0.2
	case "high":
		speedFactor = 0.3
	}

	if ac.Mode == "cooling" {
		if tempDiff < 0 {
			// 需要降温
			ac.CurrentTemp += tempDiff * speedFactor
			if ac.CurrentTemp < ac.TargetTemp {
				ac.CurrentTemp = ac.TargetTemp
			}
		}
	} else if ac.Mode == "heating" {
		if tempDiff > 0 {
			// 需要升温
			ac.CurrentTemp += tempDiff * speedFactor
			if ac.CurrentTemp > ac.TargetTemp {
				ac.CurrentTemp = ac.TargetTemp
			}
		}
	}
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
