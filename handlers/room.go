package handlers

import (
	"bupt-hotel/database"
	"bupt-hotel/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// BookRoomRequest 订房请求结构
type BookRoomRequest struct {
	RoomID     int    `json:"room_id" binding:"required"`
	ClientName string `json:"client_name" binding:"required"`
	Days       int    `json:"days" binding:"required,min=1"` // 入住天数
}

// GetAvailableRooms 获取所有空房间
func GetAvailableRooms(c *gin.Context) {
	var rooms []models.RoomInfo
	if err := database.DB.Where("state = ?", 0).Find(&rooms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取房间信息失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "获取空房间成功",
		"rooms":   rooms,
	})
}

// GetAllRooms 获取所有房间（管理员权限）
func GetAllRooms(c *gin.Context) {
	var rooms []models.RoomInfo
	if err := database.DB.Find(&rooms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取房间信息失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "获取所有房间成功",
		"rooms":   rooms,
	})
}

// BookRoom 订房
func BookRoom(c *gin.Context) {
	var req BookRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取用户信息
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	// 检查房间是否存在且为空房
	var room models.RoomInfo
	if err := database.DB.Where("room_id = ? AND state = ?", req.RoomID, 0).First(&room).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "房间不存在或已被占用",
		})
		return
	}

	// 更新房间信息
	checkinTime := time.Now()
	checkoutTime := checkinTime.AddDate(0, 0, req.Days)
	totalCost := float32(req.Days) * room.DailyRate

	room.ClientID = strconv.Itoa(userID.(int))
	room.ClientName = req.ClientName
	room.CheckinTime = checkinTime
	room.CheckoutTime = checkoutTime
	room.State = 1 // 已入住

	if err := database.DB.Save(&room).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "订房失败",
		})
		return
	}

	// 保存房间操作日志
	roomOperation := models.RoomOperation{
		RoomID:        req.RoomID,
		ClientID:      strconv.Itoa(userID.(int)),
		ClientName:    req.ClientName,
		OperationType: "checkin",
		OperationTime: checkinTime,
		CheckinTime:   checkinTime,
		CheckoutTime:  checkoutTime,
		DailyRate:     room.DailyRate,
		Deposit:       room.Deposit,
		TotalCost:     totalCost,
		ActualDays:    req.Days,
	}

	if err := database.DB.Create(&roomOperation).Error; err != nil {
		// 记录日志失败不影响主要业务流程，只记录错误
		// 可以考虑使用日志库记录这个错误
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "订房成功",
		"room_id":       room.RoomID,
		"client_name":   room.ClientName,
		"checkin_time":  room.CheckinTime,
		"checkout_time": room.CheckoutTime,
		"daily_rate":    room.DailyRate,
		"deposit":       room.Deposit,
		"total_cost":    totalCost,
		"username":      username,
	})
}

// CheckoutRoom 退房
func CheckoutRoom(c *gin.Context) {
	roomIDStr := c.Param("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的房间ID",
		})
		return
	}

	// 获取用户信息
	userID, _ := c.Get("user_id")
	identity, _ := c.Get("identity")

	// 查找房间
	var room models.RoomInfo
	query := database.DB.Where("room_id = ? AND state = ?", roomID, 1)

	// 如果不是管理员，只能退自己的房间
	if identity != "administrator" {
		query = query.Where("client_id = ?", strconv.Itoa(userID.(int)))
	}

	if err := query.First(&room).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "房间不存在或您无权操作此房间",
		})
		return
	}

	// 计算实际费用
	actualDays := int(time.Since(room.CheckinTime).Hours()/24) + 1
	if actualDays < 1 {
		actualDays = 1
	}
	actualCost := float32(actualDays) * room.DailyRate
	checkoutTime := time.Now()

	// 保存房间操作日志（在重置房间状态之前）
	roomOperation := models.RoomOperation{
		RoomID:        roomID,
		ClientID:      room.ClientID,
		ClientName:    room.ClientName,
		OperationType: "checkout",
		OperationTime: checkoutTime,
		CheckinTime:   room.CheckinTime,
		CheckoutTime:  checkoutTime,
		DailyRate:     room.DailyRate,
		Deposit:       room.Deposit,
		TotalCost:     actualCost,
		ActualDays:    actualDays,
	}

	if err := database.DB.Create(&roomOperation).Error; err != nil {
		// 记录日志失败不影响主要业务流程，只记录错误
		// 可以考虑使用日志库记录这个错误
	}

	// 重置房间状态
	room.ClientID = ""
	room.ClientName = ""
	room.CheckinTime = time.Time{}
	room.CheckoutTime = time.Time{}
	room.State = 0 // 空房

	if err := database.DB.Save(&room).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "退房失败",
		})
		return
	}

	// 关闭房间空调
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err == nil {
		ac.ACState = 0 // 关闭空调
		database.DB.Save(&ac)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "退房成功",
		"room_id":     roomID,
		"actual_days": actualDays,
		"actual_cost": actualCost,
		"deposit":     room.Deposit,
		"refund":      room.Deposit - actualCost,
	})
}

// GetMyRooms 获取我的房间
func GetMyRooms(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var rooms []models.RoomInfo
	if err := database.DB.Where("client_id = ? AND state = ?", strconv.Itoa(userID.(int)), 1).Find(&rooms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取房间信息失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "获取我的房间成功",
		"rooms":   rooms,
	})
}
