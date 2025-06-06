package handlers

import (
	"bupt-hotel/database"
	"bupt-hotel/models"
	"fmt"
	"log"
	"sync"
	"time"
)

// abs 返回整数的绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ACRequest 空调请求结构
type ACRequest struct {
	RoomID       int
	ACState      int
	Mode         string
	CurrentSpeed string
	TargetTemp   int // 目标温度*10，如245表示24.5度
	RequestTime  time.Time
	StartTime    time.Time
	Priority     int // 1: high, 2: medium, 3: low
}

// ACScheduler 空调调度器
type ACScheduler struct {
	mu           sync.RWMutex
	servingQueue []*ACRequest // 正在服务的队列（最多3个）
	waitingQueue []*ACRequest // 等待队列
	ticker       *time.Ticker
	stopChan     chan bool
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
			ticker:       time.NewTicker(time.Second), // 每秒检查一次
			stopChan:     make(chan bool),
		}
		go scheduler.run()
	})
	return scheduler
}

// 调度器运行逻辑
func (s *ACScheduler) run() {
	for {
		select {
		case <-s.ticker.C:
			s.logQueueStatus()
			s.updateTemperatures()
			s.checkTimeSlices()
			s.scheduleNext()
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
		var ac models.AirConditioner
		currentTemp := "未知"
		totalCost := float32(0)
		if err := database.DB.Where("room_id = ?", req.RoomID).First(&ac).Error; err == nil {
			currentTemp = fmt.Sprintf("%.1f°C", float32(ac.CurrentTemp)/10.0)
		}
		// 获取总费用
		database.DB.Model(&models.Detail{}).Where("room_id = ?", req.RoomID).Select("COALESCE(SUM(cost), 0)").Scan(&totalCost)
		servingInfo = append(servingInfo, fmt.Sprintf("房间%d(优先级%d,已服务%.1f分钟,当前温度%s,已花费%.2f元)", req.RoomID, req.Priority, serviceTime, currentTemp, totalCost))
	}

	// 构建等待队列信息
	waitingInfo := make([]string, 0)
	for i, req := range s.waitingQueue {
		waitTime := time.Since(req.RequestTime).Minutes()
		// 获取当前房间温度和费用
		var ac models.AirConditioner
		currentTemp := "未知"
		totalCost := float32(0)
		if err := database.DB.Where("room_id = ?", req.RoomID).First(&ac).Error; err == nil {
			currentTemp = fmt.Sprintf("%.1f°C", float32(ac.CurrentTemp)/10.0)
		}
		// 获取总费用
		database.DB.Model(&models.Detail{}).Where("room_id = ?", req.RoomID).Select("COALESCE(SUM(cost), 0)").Scan(&totalCost)
		waitingInfo = append(waitingInfo, fmt.Sprintf("房间%d(优先级%d,等待%.1f分钟,位置%d,当前温度%s,已花费%.2f元)", req.RoomID, req.Priority, waitTime, i+1, currentTemp, totalCost))
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
}

// 更新单个空调温度
func (s *ACScheduler) updateACTemperature(req *ACRequest) {
	var ac models.AirConditioner
	if err := database.DB.Where("room_id = ?", req.RoomID).First(&ac).Error; err != nil {
		return
	}

	if ac.ACState == 0 {
		return
	}

	// 计算温度变化 - 每6秒刷新一次，温度变化量为1（相当于0.1度）
	var tempChange int
	serviceSeconds := time.Since(req.StartTime).Seconds()

	// 每6秒检查一次温度变化
	if int(serviceSeconds)%6 == 0 && serviceSeconds >= 6 {
		switch req.CurrentSpeed {
		case "high":
			tempChange = 1 // 高速风每6秒变化1（0.1度）
		case "medium":
			// 中速风每12秒变化1（0.1度）
			if int(serviceSeconds)%12 == 0 {
				tempChange = 1
			}
		case "low":
			// 低速风每18秒变化1（0.1度）
			if int(serviceSeconds)%18 == 0 {
				tempChange = 1
			}
		}
	}

	// 根据模式调整温度
	if tempChange > 0 {
		oldTemp := ac.CurrentTemp
		if req.Mode == "cooling" {
			if ac.CurrentTemp > req.TargetTemp {
				ac.CurrentTemp -= tempChange
				if ac.CurrentTemp <= req.TargetTemp {
					ac.CurrentTemp = req.TargetTemp
					// 达到目标温度，进入回温模式
					s.startWarmBack(req)
				}
			}
		} else if req.Mode == "heating" {
			if ac.CurrentTemp < req.TargetTemp {
				ac.CurrentTemp += tempChange
				if ac.CurrentTemp >= req.TargetTemp {
					ac.CurrentTemp = req.TargetTemp
					// 达到目标温度，进入回温模式
					s.startWarmBack(req)
				}
			}
		}

		// 计算费用：每变化1度（10个单位）收费1元
		actualTempChange := abs(ac.CurrentTemp - oldTemp)
		if actualTempChange > 0 {
			// 每10个单位（1度）收费1元，按比例计算
			cost := float32(actualTempChange) / 10.0

			// 记录计费详情
			detail := models.Detail{
				RoomID:      req.RoomID,
				QueryTime:   time.Now(),
				StartTime:   req.StartTime,
				EndTime:     time.Now(),
				ServeTime:   float32(time.Since(req.StartTime).Minutes()),
				Speed:       req.CurrentSpeed,
				Mode:        req.Mode,
				Cost:        cost,
				Rate:        cost / float32(time.Since(req.StartTime).Minutes()),
				TempChange:  actualTempChange,
				CurrentTemp: ac.CurrentTemp,
				TargetTemp:  req.TargetTemp,
			}
			database.DB.Create(&detail)
		}

		database.DB.Save(&ac)
	}
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

	// 启动回温协程
	go s.warmBackProcess(req)
}

// 回温过程
func (s *ACScheduler) warmBackProcess(req *ACRequest) {
	ticker := time.NewTicker(12 * time.Second)
	defer ticker.Stop()

	warmBackCount := 0

	for range ticker.C {
		var ac models.AirConditioner
		if err := database.DB.Where("room_id = ?", req.RoomID).First(&ac).Error; err != nil {
			return
		}

		// 每12秒回温1个单位（0.1度）
		if req.Mode == "cooling" {
			ac.CurrentTemp += 1
		} else {
			ac.CurrentTemp -= 1
		}

		warmBackCount++
		database.DB.Save(&ac)

		// 回温1度后重新发送温控请求
		if warmBackCount >= 10 { // 10次 * 12秒 * 1单位 = 10单位（1度）
			s.AddRequest(req)
			return
		}
	}
}

// 更新关机空调的回温
func (s *ACScheduler) updateOfflineTemperatures() {
	var acs []models.AirConditioner
	database.DB.Where("ac_state = ?", 0).Find(&acs)

	// 获取当前时间的秒数，每12秒执行一次温度变化
	currentSeconds := time.Now().Unix()
	if currentSeconds%12 != 0 {
		return
	}

	for _, ac := range acs {
		// 关机时每12秒向初始温度变化1个单位（0.1度）
		if ac.CurrentTemp != ac.InitialTemp {
			if ac.CurrentTemp > ac.InitialTemp {
				ac.CurrentTemp -= 1
				if ac.CurrentTemp < ac.InitialTemp {
					ac.CurrentTemp = ac.InitialTemp
				}
			} else {
				ac.CurrentTemp += 1
				if ac.CurrentTemp > ac.InitialTemp {
					ac.CurrentTemp = ac.InitialTemp
				}
			}
			database.DB.Save(&ac)
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
		// 只有同优先级的请求才使用时间片调度
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
