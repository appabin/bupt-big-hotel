package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"bupt-hotel/database"
	"bupt-hotel/models"
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

	// 获取空调费用详单
	var acDetails []models.Detail
	var acTotalCost float32
	database.DB.Where("room_id = ?", roomID).Find(&acDetails)
	database.DB.Model(&models.Detail{}).Where("room_id = ?", roomID).Select("COALESCE(SUM(cost), 0)").Scan(&acTotalCost)

	// 关闭并重置房间空调
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err == nil {
		// 重置空调状态
		ac.ACState = 0                   // 关闭空调
		ac.CurrentTemp = ac.InitialTemp  // 重置为初始温度
		ac.TargetTemp = ac.InitialTemp   // 重置目标温度
		ac.Mode = "cooling"              // 重置为制冷模式
		ac.CurrentSpeed = "medium"       // 重置为中速
		ac.LastPowerOnTime = time.Time{} // 清空最后开机时间
		ac.SwitchCount = 0               // 重置开关次数
		database.DB.Save(&ac)
	}

	// 导出空调操作详单到Excel文件
	if len(acDetails) > 0 {
		roomName := fmt.Sprintf("房间%d", room.RoomID)
		exportACDetailsToExcel(roomName, acDetails, acTotalCost, ac)
	}

	// 清空该房间的空调费用详单记录
	database.DB.Where("room_id = ?", roomID).Delete(&models.Detail{})

	// 计算总费用（房费 + 空调费）
	totalBill := actualCost + acTotalCost
	refund := room.Deposit - totalBill

	c.JSON(http.StatusOK, gin.H{
		"message":     "退房成功",
		"room_id":     roomID,
		"actual_days": actualDays,
		"room_cost":   actualCost,
		"ac_cost":     acTotalCost,
		"total_cost":  totalBill,
		"deposit":     room.Deposit,
		"refund":      refund,
		"ac_details":  acDetails,
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

func GetAllRoomTypes(c *gin.Context) {
	// 从数据库获取所有房间类型
	var roomTypes []models.RoomType
	if err := database.DB.Find(&roomTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取房间类型信息失败",
		})
		return
	}

	// 创建结果数组，只包含必要的字段
	result := make([]RoomTypeRequest, 0, len(roomTypes))

	// 遍历房间类型，只提取需要的字段
	for _, rt := range roomTypes {
		result = append(result, RoomTypeRequest{
			Type:        rt.Type,
			ID:          rt.ID,
			Description: rt.Description,
			PriceRange:  rt.PriceRange,
			Features:    rt.Features,
		})
	}

	True := 1
	c.JSON(http.StatusOK, gin.H{
		"success": True,
		"message": "获取房间类型列表成功",
		"data":    result,
	})
}

type RoomTypeRequest struct {
	Type        string   `json:"type"`
	ID          int      `json:"id"`
	Description string   `json:"description"`
	PriceRange  string   `json:"price_range"`
	Features    []string `json:"features"`
}

// UpdateRoomType 修改指定ID的房间类型
func UpdateRoomType(c *gin.Context) {
	// 获取房间类型ID
	roomTypeID := c.Param("id")
	if roomTypeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间类型ID不能为空",
		})
		return
	}

	// 绑定请求数据
	var updateData = RoomTypeRequest{}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求数据格式错误: " + err.Error(),
		})
		return
	}

	// 查找房间类型
	var roomType models.RoomType
	if err := database.DB.First(&roomType, roomTypeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "房间类型不存在",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "查询房间类型失败",
			})
		}
		return
	}

	// 更新房间类型信息
	if updateData.Type != "" {
		roomType.Type = updateData.Type
	}
	if updateData.Description != "" {
		roomType.Description = updateData.Description
	}
	if updateData.PriceRange != "" {
		roomType.PriceRange = updateData.PriceRange
	}
	if len(updateData.Features) > 0 {
		roomType.Features = updateData.Features
	}

	// 保存更新
	if err := database.DB.Save(&roomType).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新房间类型失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "房间类型更新成功",
		"room_type": roomType,
	})
}

// 简化的房间信息结构体，只包含room_id和price字段
type SimpleRoomInfo struct {
	RoomID       int     `json:"room_id"`
	Price        float32 `json:"price"`
	State        int     `json:"state"`
	RoomTypeName string  `json:"room_type"`
}

// GetRoomsByType 通过房间类型ID获取对应类型的所有房间
func GetRoomsByType(c *gin.Context) {
	// 获取房间类型ID
	typeID := c.Param("type_id")
	if typeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间类型ID不能为空",
		})
		return
	}

	// 查询该类型的所有未入住房间
	var rooms []models.RoomInfo
	if err := database.DB.Where("room_type_id = ? AND state = ?", typeID, 0).Find(&rooms).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取房间信息失败",
		})
		return
	}

	var typeName string
	// 查询房间类型名称
	if err := database.DB.Model(&models.RoomType{}).Where("id =?", typeID).Pluck("type", &typeName).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取房间类型名称失败",
		})
		return
	}

	// 如果没有找到房间，返回空数组而不是错误
	if len(rooms) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "未找到该类型的房间",
			"rooms":   []SimpleRoomInfo{},
		})
		return
	}

	// 创建简化的房间信息数组
	simpleRooms := make([]SimpleRoomInfo, 0, len(rooms))
	for _, room := range rooms {
		simpleRooms = append(simpleRooms, SimpleRoomInfo{
			RoomID:       room.RoomID,
			Price:        room.DailyRate,
			State:        room.State,
			RoomTypeName: typeName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "获取房间信息成功",
		"rooms":   simpleRooms,
		"success": 1,
	})
}

// exportACDetailsToExcel 导出空调操作详单到Excel文件
func exportACDetailsToExcel(roomName string, details []models.Detail, totalCost float32, ac models.AirConditioner) {
	// 创建新的Excel文件
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("关闭Excel文件失败: %v\n", err)
		}
	}()

	// 设置工作表名称
	sheetName := "空调费用详单"
	f.SetSheetName("Sheet1", sheetName)

	// 设置表头
	headers := []string{"序号", "服务时间(分钟)", "费用(元)", "费率", "风速", "模式", "温度变化", "当前温度", "目标温度", "记录时间"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	// 填充数据
	for i, detail := range details {
		row := i + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), detail.ServeTime)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), fmt.Sprintf("%.2f", detail.Cost))
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), detail.Rate)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), detail.Speed)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), detail.Mode)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), detail.TempChange)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), fmt.Sprintf("%.1f°C", float64(detail.CurrentTemp)/10.0))
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), fmt.Sprintf("%.1f°C", float64(detail.TargetTemp)/10.0))
		f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), detail.QueryTime.Format("2006-01-02 15:04:05"))
	}

	// 计算总开启时长（分钟）
	totalServeTime := float32(0)
	for _, detail := range details {
		totalServeTime += detail.ServeTime
	}

	// 添加总结信息
	summaryRow := len(details) + 3
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", summaryRow), "总计")
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", summaryRow), fmt.Sprintf("%.2f元", totalCost))

	// 添加总开启时长
	totalTimeRow := summaryRow + 1
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", totalTimeRow), "总开启时长")
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", totalTimeRow), fmt.Sprintf("%.1f分钟", totalServeTime))

	// 添加开机次数
	switchCountRow := totalTimeRow + 1
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", switchCountRow), "开机次数")
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", switchCountRow), fmt.Sprintf("%d次", ac.SwitchCount))

	// 设置列宽
	f.SetColWidth(sheetName, "A", "A", 8)  // 序号
	f.SetColWidth(sheetName, "B", "B", 15) // 服务时间
	f.SetColWidth(sheetName, "C", "C", 12) // 费用
	f.SetColWidth(sheetName, "D", "D", 8)  // 费率
	f.SetColWidth(sheetName, "E", "E", 10) // 风速
	f.SetColWidth(sheetName, "F", "F", 10) // 模式
	f.SetColWidth(sheetName, "G", "G", 12) // 温度变化
	f.SetColWidth(sheetName, "H", "H", 12) // 当前温度
	f.SetColWidth(sheetName, "I", "I", 12) // 目标温度
	f.SetColWidth(sheetName, "J", "J", 20) // 记录时间

	// 保存文件
	filename := fmt.Sprintf("%s_空调费用详单_%s.xlsx", roomName, time.Now().Format("20060102_150405"))
	if err := f.SaveAs(filename); err != nil {
		fmt.Printf("保存Excel文件失败: %v\n", err)
	} else {
		fmt.Printf("空调费用详单已保存到: %s\n", filename)
	}
}
