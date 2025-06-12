package handlers

import (
	"bupt-hotel/database"
	"bupt-hotel/models"
	"fmt"
	"log"
	"sync"
	"time"
)

// ACRequest 空调请求结构
type ACRequest struct {
	BillID        int
	RoomID        int
	ACID          int
	ACState       int //0-运行 1-暂停服务 2-停机
	Mode          string
	CurrentSpeed  string
	TargetTemp    int
	RequestTime   time.Time
	StartTime     time.Time
	Priority      int // 1: high, 2: medium, 3: low
	OperationType int // 0: 开机, 1: 关机, 2: 调温
}

// ACScheduler 空调调度器
type ACScheduler struct {
	mu           sync.RWMutex
	servingQueue []*ACRequest // 正在服务的队列（最多3个）
	waitingQueue []*ACRequest // 等待队列
	ticker       *time.Ticker
	stopChan     chan bool
	warmBackMap  map[int]*ACRequest // 回温程序中的空调
}

// 全局调度器
var (
	scheduler *ACScheduler
	once      sync.Once
)

// GetScheduler 获取调度器单例
func GetScheduler() *ACScheduler {
	once.Do(func() {
		scheduler = &ACScheduler{
			servingQueue: make([]*ACRequest, 0, 3),
			waitingQueue: make([]*ACRequest, 0),
			ticker:       time.NewTicker(6 * time.Second), // 每6秒检查一次
			stopChan:     make(chan bool),
			warmBackMap:  make(map[int]*ACRequest),
		}
		go scheduler.run()
	})
	return scheduler
}

// 调度器运行逻辑
func (s *ACScheduler) run() {
	scheduleCounter := 0
	for {
		select {
		case <-s.ticker.C:
			s.updateTemperatures()
			s.logQueueStatus()
			scheduleCounter++
			// 每分钟进行一次调度（10次6秒 = 60秒）
			if scheduleCounter >= 10 {
				s.checkTimeSlices()
				s.scheduleNext()
				scheduleCounter = 0
			}
		case <-s.stopChan:
			return
		}
	}
}

// 输出队列状态日志
func (s *ACScheduler) logQueueStatus() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 构建服务队列信息
	servingInfo := make([]string, 0)
	for _, req := range s.servingQueue {
		serviceTime := time.Since(req.StartTime).Minutes()
		// 获取当前房间温度和费用
		currentTemp := "未知"
		totalCost := float32(0)
		
		// 从最新的状态记录获取当前温度
		var lastDetail models.AirConditionerDetail
		if err := database.DB.Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Order("created_at DESC").First(&lastDetail).Error; err == nil {
			currentTemp = fmt.Sprintf("%.1f°C", float32(lastDetail.CurrentTemp)/10.0)
		}
		// 获取总费用
		database.DB.Model(&models.AirConditionerDetail{}).Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Select("COALESCE(SUM(current_cost), 0)").Scan(&totalCost)
		servingInfo = append(servingInfo, fmt.Sprintf("房间%d(订单%d,优先级%d,已服务%.1f分钟,当前温度%s,已花费%.2f元)", req.RoomID, req.BillID, req.Priority, serviceTime, currentTemp, totalCost))
	}

	// 构建等待队列信息
	waitingInfo := make([]string, 0)
	for i, req := range s.waitingQueue {
		waitTime := time.Since(req.RequestTime).Minutes()
		// 获取当前房间温度和费用
		currentTemp := "未知"
		totalCost := float32(0)
		
		// 从最新的状态记录获取当前温度
		var lastDetail models.AirConditionerDetail
		if err := database.DB.Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Order("created_at DESC").First(&lastDetail).Error; err == nil {
			currentTemp = fmt.Sprintf("%.1f°C", float32(lastDetail.CurrentTemp)/10.0)
		}
		// 获取总费用
		database.DB.Model(&models.AirConditionerDetail{}).Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Select("COALESCE(SUM(current_cost), 0)").Scan(&totalCost)
		waitingInfo = append(waitingInfo, fmt.Sprintf("房间%d(订单%d,优先级%d,等待%.1f分钟,位置%d,当前温度%s,已花费%.2f元)", req.RoomID, req.BillID, req.Priority, waitTime, i+1, currentTemp, totalCost))
	}

	// 输出日志
	if len(servingInfo) == 0 && len(waitingInfo) == 0 {
		log.Printf("[调度器状态] 服务队列: 空, 等待队列: 空")
	} else {
		servingStr := "空"
		if len(servingInfo) > 0 {
			servingStr = fmt.Sprintf("%v", servingInfo)
		}
		waitingStr := "空"
		if len(waitingInfo) > 0 {
			waitingStr = fmt.Sprintf("%v", waitingInfo)
		}
		log.Printf("[调度器状态] 服务队列(%d): %s, 等待队列(%d): %s", len(s.servingQueue), servingStr, len(s.waitingQueue), waitingStr)
	}
}

// 更新所有空调温度
func (s *ACScheduler) updateTemperatures() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新正在服务的空调温度
	for _, req := range s.servingQueue {
		s.updateACTemperature(req)
	}

	// 更新所有关机空调的回温
	s.updateOfflineTemperatures()

	// 更新回温程序中的空调
	s.updateWarmBackTemperatures()
}

// 更新单个空调温度
func (s *ACScheduler) updateACTemperature(req *ACRequest) {
	// 获取空调基础信息
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", req.RoomID).First(&ac).Error; err != nil {
		return
	}

	// 获取最新的操作记录
	var operation models.AirConditionerOperation
	if err := database.DB.Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Order("created_at DESC").First(&operation).Error; err != nil {
		return
	}

	// 如果是关机状态，不进行温度更新
	if operation.OperationState == 1 {
		return
	}

	// 计算温度变化 - 每6秒刷新一次
	var tempChange int
	serviceSeconds := time.Since(req.StartTime).Seconds()

	// 根据风速计算温度变化
	switch req.CurrentSpeed {
	case "high":
		// 高风：每分钟1度，每6秒0.1度
		tempChange = 1
	case "medium":
		// 中风：每2分钟1度，每12秒0.1度
		if int(serviceSeconds)%12 == 0 && serviceSeconds >= 12 {
			tempChange = 1
		}
	case "low":
		// 低风：每3分钟1度，每18秒0.1度
		if int(serviceSeconds)%18 == 0 && serviceSeconds >= 18 {
			tempChange = 1
		}
	}

	// 获取当前温度（从最新的状态记录或操作记录）
	currentTemp := operation.CurrentTemp
	var lastDetail models.AirConditionerDetail
	if err := database.DB.Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Order("created_at DESC").First(&lastDetail).Error; err == nil {
		currentTemp = lastDetail.CurrentTemp
	}

	// 根据模式调整温度
	if tempChange > 0 {
		oldTemp := currentTemp
		newTemp := currentTemp
		
		if req.Mode == "cooling" {
			if currentTemp > req.TargetTemp {
				newTemp = currentTemp - tempChange
				if newTemp <= req.TargetTemp {
					newTemp = req.TargetTemp
					// 达到目标温度，进入回温模式
					s.startWarmBack(req)
				}
			}
		} else if req.Mode == "heating" {
			if currentTemp < req.TargetTemp {
				newTemp = currentTemp + tempChange
				if newTemp >= req.TargetTemp {
					newTemp = req.TargetTemp
					// 达到目标温度，进入回温模式
					s.startWarmBack(req)
				}
			}
		}

		// 计算费用：温度变化量等于消费金额
		actualTempChange := abs(newTemp - oldTemp)
		if actualTempChange > 0 {
			// 每变化0.1度收费0.1元
			cost := float32(actualTempChange) / 10.0

			// 获取总费用
			var totalCost float32
			database.DB.Model(&models.AirConditionerDetail{}).Where("room_id = ? AND bill_id = ?", req.RoomID, req.BillID).Select("COALESCE(SUM(current_cost), 0)").Scan(&totalCost)
			totalCost += cost

			// 创建状态记录
			detail := models.AirConditionerDetail{
				BillID:          req.BillID,
				RoomID:          req.RoomID,
				AcID:            req.ACID,
				OperationType:   0, // 系统自动记录
				ACStatus:        0, // 运行状态
				Speed:           req.CurrentSpeed,
				Mode:            req.Mode,
				TargetTemp:      req.TargetTemp,
				EnvironmentTemp: ac.EnvironmentTemp,
				CurrentTemp:     newTemp,
				CurrentCost:     cost,
				TotalCost:       totalCost,
				QueryTime:       time.Now(),
				StartTime:       req.StartTime,
				EndTime:         time.Now(),
				ServeTime:       float32(time.Since(req.StartTime).Seconds()) / 60.0,
				QueueWaitTime:   0,
				Rate:            1.0,
				TempChange:      actualTempChange,
			}
			database.DB.Create(&detail)
		}
	}
}

// abs 返回整数的绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// 开始回温程序
func (s *ACScheduler) startWarmBack(req *ACRequest) {
	// 从服务队列中移除
	for i, r := range s.servingQueue {
		if r.RoomID == req.RoomID {
			s.servingQueue = append(s.servingQueue[:i], s.servingQueue[i+1:]...)
			break
		}
	}

	// 添加到回温程序
	req.StartTime = time.Now() // 重置开始时间用于回温计时
	s.warmBackMap[req.RoomID] = req
}

// 更新回温程序中的空调温度
func (s *ACScheduler) updateWarmBackTemperatures() {
	currentTime := time.Now()
	for roomID, req := range s.warmBackMap {
		// 检查是否已经回温2分钟
		if currentTime.Sub(req.StartTime) >= 2*time.Minute {
			// 重新加入调度队列
			req.RequestTime = currentTime
			s.AddRequest(req)
			delete(s.warmBackMap, roomID)
			continue
		}

		// 每分钟回温0.5度
		if int(currentTime.Sub(req.StartTime).Seconds())%60 == 0 && currentTime.Sub(req.StartTime).Seconds() >= 60 {
			// 获取空调基础信息
			var ac models.AirConditioner
			if err := database.DB.Where("room_id = ?", roomID).First(&ac).Error; err != nil {
				continue
			}

			// 获取当前温度
			currentTemp := ac.EnvironmentTemp
			var lastDetail models.AirConditionerDetail
			if err := database.DB.Where("room_id = ? AND bill_id = ?", roomID, req.BillID).Order("created_at DESC").First(&lastDetail).Error; err == nil {
				currentTemp = lastDetail.CurrentTemp
			}

			// 向初始温度回温0.5度（5个单位）
			newTemp := currentTemp
			if currentTemp > ac.EnvironmentTemp {
				newTemp = currentTemp - 5
				if newTemp < ac.EnvironmentTemp {
					newTemp = ac.EnvironmentTemp
				}
			} else if currentTemp < ac.EnvironmentTemp {
				newTemp = currentTemp + 5
				if newTemp > ac.EnvironmentTemp {
					newTemp = ac.EnvironmentTemp
				}
			}

			// 创建回温状态记录
			if newTemp != currentTemp {
				detail := models.AirConditionerDetail{
					BillID:          req.BillID,
					RoomID:          roomID,
					AcID:            req.ACID,
					OperationType:   3, // 回温程序
					ACStatus:        1, // 暂停服务状态
					Speed:           req.CurrentSpeed,
					Mode:            req.Mode,
					TargetTemp:      req.TargetTemp,
					EnvironmentTemp: ac.EnvironmentTemp,
					CurrentTemp:     newTemp,
					CurrentCost:     0, // 回温不产生费用
					TotalCost:       0,
					QueryTime:       currentTime,
					StartTime:       req.StartTime,
					EndTime:         currentTime,
					ServeTime:       0,
					QueueWaitTime:   0,
					Rate:            0,
					TempChange:      abs(newTemp - currentTemp),
				}
				database.DB.Create(&detail)
			}
		}
	}
}

// 更新关机空调的回温
func (s *ACScheduler) updateOfflineTemperatures() {
	// 获取所有关机状态的操作记录
	var operations []models.AirConditionerOperation
	database.DB.Where("operation_state = ?", 1).Find(&operations)

	// 每分钟执行一次温度变化
	currentSeconds := time.Now().Unix()
	if currentSeconds%60 != 0 {
		return
	}

	for _, operation := range operations {
		// 获取空调基础信息
		var ac models.AirConditioner
		if err := database.DB.Where("room_id = ?", operation.RoomID).First(&ac).Error; err != nil {
			continue
		}

		// 获取当前温度（从最新的状态记录）
		currentTemp := operation.CurrentTemp
		var lastDetail models.AirConditionerDetail
		if err := database.DB.Where("room_id = ? AND bill_id = ?", operation.RoomID, operation.BillID).Order("created_at DESC").First(&lastDetail).Error; err == nil {
			currentTemp = lastDetail.CurrentTemp
		}

		// 关机时每分钟向初始温度回温0.5度（5个单位）
		if currentTemp != ac.EnvironmentTemp {
			newTemp := currentTemp
			if currentTemp > ac.EnvironmentTemp {
				newTemp = currentTemp - 5
				if newTemp < ac.EnvironmentTemp {
					newTemp = ac.EnvironmentTemp
				}
			} else {
				newTemp = currentTemp + 5
				if newTemp > ac.EnvironmentTemp {
					newTemp = ac.EnvironmentTemp
				}
			}

			// 创建回温状态记录
			if newTemp != currentTemp {
				detail := models.AirConditionerDetail{
					BillID:          operation.BillID,
					RoomID:          operation.RoomID,
					AcID:            operation.AcID,
					OperationType:   1, // 关机回温
					ACStatus:        2, // 停机状态
					Speed:           operation.Speed,
					Mode:            operation.Mode,
					TargetTemp:      operation.TargetTemp,
					EnvironmentTemp: ac.EnvironmentTemp,
					CurrentTemp:     newTemp,
					CurrentCost:     0, // 关机不产生费用
					TotalCost:       0,
					QueryTime:       time.Now(),
					StartTime:       time.Now(),
					EndTime:         time.Now(),
					ServeTime:       0,
					QueueWaitTime:   0,
					Rate:            0,
					TempChange:      abs(newTemp - currentTemp),
				}
				database.DB.Create(&detail)
			}
		}
	}
}

// 检查时间片
func (s *ACScheduler) checkTimeSlices() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for i := len(s.servingQueue) - 1; i >= 0; i-- {
		req := s.servingQueue[i]
		// 检查是否有同优先级的请求在等待
		hasSamePriorityWaiting := false
		for _, waitingReq := range s.waitingQueue {
			if waitingReq.Priority == req.Priority {
				hasSamePriorityWaiting = true
				break
			}
		}

		// 只有在有同优先级等待且服务时长达到2分钟时才进行时间片切换
		if hasSamePriorityWaiting && now.Sub(req.StartTime) >= 2*time.Minute {
			s.servingQueue = append(s.servingQueue[:i], s.servingQueue[i+1:]...)
			req.RequestTime = now // 重新设置请求时间
			s.waitingQueue = append(s.waitingQueue, req)
		}
	}
}

// 调度下一个请求
func (s *ACScheduler) scheduleNext() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果服务队列未满且有等待请求
	for len(s.servingQueue) < 3 && len(s.waitingQueue) > 0 {
		// 按优先级排序等待队列
		s.sortWaitingQueue()

		// 获取当前服务队列中的最高优先级
		currentHighestPriority := s.getCurrentHighestPriority()

		// 获取等待队列中的最高优先级
		waitingHighestPriority := s.waitingQueue[0].Priority

		// 只有当等待队列的最高优先级不低于当前服务队列的最高优先级时才调度
		if waitingHighestPriority <= currentHighestPriority {
			// 取出优先级最高的请求
			req := s.waitingQueue[0]
			s.waitingQueue = s.waitingQueue[1:]

			// 添加到服务队列
			req.StartTime = time.Now()
			s.servingQueue = append(s.servingQueue, req)
		} else {
			// 如果等待队列的优先级更低，则不调度，等待当前高优先级完成
			break
		}
	}
}

// 按优先级排序等待队列
func (s *ACScheduler) sortWaitingQueue() {
	// 简单的冒泡排序，按优先级和请求时间排序
	for i := 0; i < len(s.waitingQueue)-1; i++ {
		for j := 0; j < len(s.waitingQueue)-1-i; j++ {
			// 优先级高的在前，优先级相同时先请求的在前
			if s.waitingQueue[j].Priority > s.waitingQueue[j+1].Priority ||
				(s.waitingQueue[j].Priority == s.waitingQueue[j+1].Priority &&
					s.waitingQueue[j].RequestTime.After(s.waitingQueue[j+1].RequestTime)) {
				s.waitingQueue[j], s.waitingQueue[j+1] = s.waitingQueue[j+1], s.waitingQueue[j]
			}
		}
	}
}

// 获取当前服务队列中的最高优先级
func (s *ACScheduler) getCurrentHighestPriority() int {
	if len(s.servingQueue) == 0 {
		return 999 // 如果服务队列为空，返回最低优先级
	}

	highestPriority := s.servingQueue[0].Priority
	for _, req := range s.servingQueue {
		if req.Priority < highestPriority {
			highestPriority = req.Priority
		}
	}
	return highestPriority
}

// AddRequest 添加请求到调度器
func (s *ACScheduler) AddRequest(req *ACRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已经在队列中
	for _, r := range s.servingQueue {
		if r.RoomID == req.RoomID {
			return // 已经在服务队列中
		}
	}
	for _, r := range s.waitingQueue {
		if r.RoomID == req.RoomID {
			return // 已经在等待队列中
		}
	}

	// 设置优先级
	switch req.CurrentSpeed {
	case "high":
		req.Priority = 1
	case "medium":
		req.Priority = 2
	case "low":
		req.Priority = 3
	}

	// 检查是否可以抢占低优先级的服务
	if len(s.servingQueue) < 3 {
		// 服务队列未满，直接加入
		req.StartTime = time.Now()
		s.servingQueue = append(s.servingQueue, req)
	} else {
		// 服务队列已满，检查是否可以抢占
		lowestPriorityInServing := s.getLowestPriorityInServing()
		if req.Priority < lowestPriorityInServing {
			// 新请求优先级更高，抢占最低优先级的服务
			s.preemptLowestPriority(req)
		} else {
			// 无法抢占，加入等待队列
			req.RequestTime = time.Now()
			s.waitingQueue = append(s.waitingQueue, req)
		}
	}
}

// 获取服务队列中的最低优先级
func (s *ACScheduler) getLowestPriorityInServing() int {
	if len(s.servingQueue) == 0 {
		return 0
	}

	lowestPriority := s.servingQueue[0].Priority
	for _, req := range s.servingQueue {
		if req.Priority > lowestPriority {
			lowestPriority = req.Priority
		}
	}
	return lowestPriority
}

// 抢占最低优先级的服务
func (s *ACScheduler) preemptLowestPriority(newReq *ACRequest) {
	// 找到最低优先级的请求
	lowestPriorityIndex := -1
	lowestPriority := 0

	for i, req := range s.servingQueue {
		if req.Priority > lowestPriority {
			lowestPriority = req.Priority
			lowestPriorityIndex = i
		}
	}

	if lowestPriorityIndex != -1 {
		// 将被抢占的请求移到等待队列
		preemptedReq := s.servingQueue[lowestPriorityIndex]
		preemptedReq.RequestTime = time.Now()
		s.servingQueue = append(s.servingQueue[:lowestPriorityIndex], s.servingQueue[lowestPriorityIndex+1:]...)
		s.waitingQueue = append(s.waitingQueue, preemptedReq)

		// 将新请求加入服务队列
		newReq.StartTime = time.Now()
		s.servingQueue = append(s.servingQueue, newReq)
	}
}

// RemoveRequest 移除请求
func (s *ACScheduler) RemoveRequest(roomID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 从服务队列中移除
	for i, req := range s.servingQueue {
		if req.RoomID == roomID {
			s.servingQueue = append(s.servingQueue[:i], s.servingQueue[i+1:]...)
			return
		}
	}

	// 从等待队列中移除
	for i, req := range s.waitingQueue {
		if req.RoomID == roomID {
			s.waitingQueue = append(s.waitingQueue[:i], s.waitingQueue[i+1:]...)
			return
		}
	}

	// 从回温程序中移除
	delete(s.warmBackMap, roomID)
}

// GetServingQueue 获取服务队列（只读）
func (s *ACScheduler) GetServingQueue() []*ACRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*ACRequest(nil), s.servingQueue...)
}

// GetWaitingQueue 获取等待队列（只读）
func (s *ACScheduler) GetWaitingQueue() []*ACRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*ACRequest(nil), s.waitingQueue...)
}
