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

	// 生成账单号：时间戳+房间号
	billIDStr := fmt.Sprintf("%d%03d", checkinTime.Unix(), req.RoomID)
	billID, _ := strconv.Atoi(billIDStr)

	// 保存房间操作日志
	roomOperation := models.RoomOperation{
		RoomID:        req.RoomID,
		BillID:        billID,
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
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "订房成功",
		"bill_id":       billID,
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

	// 获取当前入住的账单号
	var checkinOperation models.RoomOperation
	if err := database.DB.Where("room_id = ? AND operation_type = 'checkin'", roomID).Order("operation_time DESC").First(&checkinOperation).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "无法找到入住记录",
		})
		return
	}
	billID := checkinOperation.BillID

	// 计算实际费用
	actualDays := int(time.Since(room.CheckinTime).Hours()/24) + 1
	if actualDays < 1 {
		actualDays = 1
	}
	actualCost := float32(actualDays) * room.DailyRate
	checkoutTime := time.Now()

	// 获取空调操作记录
	var acOperations []models.AirConditionerOperation
	if err := database.DB.Where("room_id = ? AND bill_id = ?", roomID, billID).Find(&acOperations).Error; err != nil {
		// 如果没有空调操作记录，继续退房流程
	}

	// 生成Excel文件
	excelFile, err := generateACReportExcel(billID, roomID, acOperations)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "生成空调使用报告失败: " + err.Error(),
		})
		return
	}

	// 保存房间操作日志（在重置房间状态之前）
	roomOperation := models.RoomOperation{
		RoomID:        roomID,
		BillID:        billID,
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

	// 设置响应头并返回Excel文件
	filename := fmt.Sprintf("空调使用详单_%d_%d.xlsx", checkinOperation.BillID, roomID)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Transfer-Encoding", "binary")

	// 将Excel文件写入响应
	if err := excelFile.Write(c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "下载文件失败",
		})
		return
	}
}

// generateACReportExcel 生成空调使用报告Excel文件
func generateACReportExcel(billID int, roomID int, acOperations []models.AirConditionerOperation) (*excelize.File, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	// 设置工作表名称
	sheetName := "空调使用详单"
	f.SetSheetName("Sheet1", sheetName)

	// 设置标题
	f.SetCellValue(sheetName, "A1", "空调使用报告")
	f.SetCellValue(sheetName, "A2", fmt.Sprintf("账单号: %d", billID))
	f.SetCellValue(sheetName, "A3", fmt.Sprintf("房间号: %d", roomID))
	f.SetCellValue(sheetName, "A4", fmt.Sprintf("生成时间: %s", time.Now().Format("2006-01-02 15:04:05")))

	// 空调操作记录表头
	f.SetCellValue(sheetName, "A6", "空调操作记录")
	f.SetCellValue(sheetName, "A7", "序号")
	f.SetCellValue(sheetName, "B7", "操作时间")
	f.SetCellValue(sheetName, "C7", "操作类型")
	f.SetCellValue(sheetName, "D7", "目标温度")
	f.SetCellValue(sheetName, "E7", "风速")
	f.SetCellValue(sheetName, "F7", "模式")

	// 填充空调操作数据
	row := 8
	for i, op := range acOperations {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), op.CreatedAt.Format("2006-01-02 15:04:05"))

		// 将操作状态数字转换为中文描述
		var operationDesc string
		switch op.OperationState {
		case 0:
			operationDesc = "开机"
		case 1:
			operationDesc = "关机"
		case 2:
			operationDesc = "调温"
		default:
			operationDesc = "未知"
		}
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), operationDesc)

		// 目标温度除以10显示实际温度
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), float32(op.TargetTemp)/10.0)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), op.Speed)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), op.Mode)

		row++
	}

	// 空调详细记录表头
	row += 2
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "空调状态详细记录")
	row++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "序号")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), "记录时间")
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "当前温度")
	f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), "目标温度")
	f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), "风速")
	f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), "模式")

	f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), "当前费用")
	f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), "总费用")
	row++

	// 添加汇总信息
	row += 2
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "汇总信息")
	row++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("总操作次数: %d", len(acOperations)))
	row++

	// 设置列宽
	f.SetColWidth(sheetName, "A", "A", 8)
	f.SetColWidth(sheetName, "B", "B", 20)
	f.SetColWidth(sheetName, "C", "I", 12)

	return f, nil
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
