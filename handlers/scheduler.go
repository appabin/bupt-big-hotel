package handlers

import (
	"bupt-hotel/database"
	"bupt-hotel/models"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ACScheduler 空调调度器框架
type ACScheduler struct {
	mu           sync.RWMutex
	schedulers   map[int]*models.Scheduler // ACID -> Scheduler
	servingQueue []*models.Scheduler       // 当前服务队列
	bufferQueue  []*models.Scheduler       // 缓冲队列
	warmingQueue []*models.Scheduler       // 回温队列

	isRunning bool         // 调度器是否正在运行
	stopChan  chan bool    // 停止信号
	ticker    *time.Ticker // 定时器

	// 新增时间片相关属性
	tickCount       int  // 当前tick计数
	currentPriority int  // 当前时间片调度优先级，初始为0
	firstACAdded    bool // 是否已添加第一个空调
}

var (
	schedulerInstance *ACScheduler
	schedulerOnce     sync.Once
)

// GetScheduler 获取调度器单例
func GetScheduler() *ACScheduler {
	schedulerOnce.Do(func() {
		schedulerInstance = &ACScheduler{
			schedulers:      make(map[int]*models.Scheduler),
			servingQueue:    make([]*models.Scheduler, 0),
			bufferQueue:     make([]*models.Scheduler, 0),
			warmingQueue:    make([]*models.Scheduler, 0),
			isRunning:       false,
			stopChan:        make(chan bool),
			tickCount:       0,
			currentPriority: 0,
			firstACAdded:    false,
		}
	})
	return schedulerInstance
}

// AddRequest 添加调度请求
func (s *ACScheduler) AddRequest(scheduler *models.Scheduler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 添加到调度器映射
	s.schedulers[scheduler.ACID] = scheduler

	// 如果是第一个空调，直接加入服务队列并启动调度器
	if !s.firstACAdded {
		s.servingQueue = append(s.servingQueue, scheduler)
		s.bufferQueue = append(s.bufferQueue, scheduler)
		s.firstACAdded = true
		if !s.isRunning {
			go s.StartScheduler()
		}
		log.Printf("第一个空调加入服务队列: 空调ID %d", scheduler.ACID)
	} else {
		// 先查询回温队列中是否有对应空调存在
		found := false
		for i, warmingScheduler := range s.warmingQueue {
			if warmingScheduler.ACID == scheduler.ACID {
				// 找到对应空调，修改其状态并转移至缓冲队列
				warmingScheduler.ACState = 1 // 设置为等待状态
				warmingScheduler.TargetTemp = scheduler.TargetTemp
				warmingScheduler.CurrentSpeed = scheduler.CurrentSpeed
				warmingScheduler.Priority = scheduler.Priority
				s.bufferQueue = append(s.bufferQueue, warmingScheduler)
				// 从回温队列中移除
				s.warmingQueue = append(s.warmingQueue[:i], s.warmingQueue[i+1:]...)
				log.Printf("空调ID %d 从回温队列转移至缓冲队列", scheduler.ACID)
				found = true
				break
			}
		}

		if !found {
			// 回温队列中没有找到，直接加入缓冲队列
			s.bufferQueue = append(s.bufferQueue, scheduler)
			log.Printf("空调加入缓冲队列: 空调ID %d", scheduler.ACID)
		}
	}
}

// UpdateACInBuffer 更新队列中的空调参数
func (s *ACScheduler) UpdateACInBuffer(acID int, targetTemp int, speed string, priority int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 在缓冲队列中查找对应的空调
	for _, scheduler := range s.bufferQueue {
		if scheduler.ACID == acID {
			// 更新目标温度和风速
			scheduler.TargetTemp = targetTemp
			scheduler.CurrentSpeed = speed
			scheduler.Priority = priority
			log.Printf("调温操作：更新缓冲队列中空调ID %d 的目标温度为 %d°C，风速为 %s", acID, targetTemp, speed)
			return true
		}
	}

	// 在回温队列中查找对应的空调
	for _, scheduler := range s.warmingQueue {
		if scheduler.ACID == acID {
			// 更新目标温度和风速
			scheduler.TargetTemp = targetTemp
			scheduler.CurrentSpeed = speed
			scheduler.Priority = priority
			log.Printf("调温操作：更新回温队列中空调ID %d 的目标温度为 %d°C，风速为 %s", acID, targetTemp, speed)
			return true
		}
	}

	return false
}

// RemoveRequest 移除调度请求
func (s *ACScheduler) RemoveRequest(acID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 在服务队列中查找
	for _, scheduler := range s.servingQueue {
		if scheduler.ACID == acID {
			scheduler.ACState = 2
			log.Printf("空调ID %d 在服务队列中找到，状态设置为2（关机回温）", acID)
			// 注意：这里只改变状态，不返回，继续查找其他队列
		}
	}

	// 在缓冲队列中查找
	for _, scheduler := range s.bufferQueue {
		if scheduler.ACID == acID {
			scheduler.ACState = 2
			log.Printf("空调ID %d 在缓冲队列中找到，状态设置为2（关机回温）", acID)
			return
		}
	}

	// 在回温队列中查找
	for _, scheduler := range s.warmingQueue {
		if scheduler.ACID == acID {
			scheduler.ACState = 2
			log.Printf("空调ID %d 在回温队列中找到，状态设置为2（关机回温）", acID)
			return
		}
	}

	log.Printf("未找到空调ID %d", acID)
}

// StartScheduler 启动调度器
func (s *ACScheduler) StartScheduler() {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.ticker = time.NewTicker(3 * time.Second) // 每6秒执行一次（trick值为6）
	s.mu.Unlock()

	log.Println("空调调度器已启动，tick间隔为3秒")

	for {
		select {
		case <-s.ticker.C:
			s.scheduleAirConditioners()
		case <-s.stopChan:
			log.Println("空调调度器已停止")
			return
		}

	}
}

// scheduleAirConditioners 执行刷新操作
func (s *ACScheduler) scheduleAirConditioners() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 增加tick计数
	s.tickCount++

	// 每个tick都刷新当前温度并输出日志
	s.refreshTemperature()

	// 刷新回温队列
	s.refreshWarmingQueue()

	// 检查是否需要进行排序（每10个tick的第9个tick，即10*n-1）
	if s.tickCount%10 == 9 {

		log.Printf("第%d个tick，开始对缓冲队列进行排序", s.tickCount+1)
		s.UpdateBufferQueue()
		s.updateWarmingQueue()
		s.sortBufferQueue()
		s.updateServingQueue()
	}

	// 记录当前状态
	log.Printf("Tick %d - 服务队列: %d, 缓冲队列: %d, 回温队列: %d, 总请求: %d",
		s.tickCount, len(s.servingQueue), len(s.bufferQueue), len(s.warmingQueue), len(s.schedulers))

	// 在刷新操作结束后保存空调状态到数据库
	s.saveACStatesToDB()

}

// UpdateBufferQueue 将服务队列中的变化更新到缓冲队列中
func (s *ACScheduler) UpdateBufferQueue() {
	// 遍历服务队列，将状态变化同步到缓冲队列中对应的空调
	for _, servingAC := range s.servingQueue {
		// 在缓冲队列中查找对应的空调
		for _, bufferAC := range s.bufferQueue {
			if bufferAC.ACID == servingAC.ACID {
				// 同步状态信息
				bufferAC.CurrentTemp = servingAC.CurrentTemp
				bufferAC.CurrentCost = servingAC.CurrentCost
				bufferAC.TotalCost = servingAC.TotalCost
				bufferAC.CurrentRunningTime = servingAC.CurrentRunningTime
				bufferAC.ACState = servingAC.ACState
				bufferAC.RunningTime = servingAC.RunningTime
				bufferAC.RoundRobinCount = servingAC.RoundRobinCount
				log.Printf("更新缓冲队列中空调ID %d: 温度=%d°C, 状态=%d, 当前费用=%d, 总费用=%d, 运行时间=%d",
					bufferAC.ACID, bufferAC.CurrentTemp, bufferAC.ACState, bufferAC.CurrentCost, bufferAC.TotalCost, bufferAC.RunningTime)
				break
			}
		}
	}

	log.Printf("完成服务队列状态同步到缓冲队列")
}

// refreshTemperature 刷新所有空调的当前温度
func (s *ACScheduler) refreshTemperature() {
	// 刷新温度只对服务队列进行刷新
	for _, scheduler := range s.servingQueue {
		// 计算温度变化量
		var tempChange int
		switch scheduler.CurrentSpeed {
		case "high":
			// 高风时每tick变化0.1度
			tempChange = 1
		case "medium":
			// 中风时每两个tick变化0.1度
			if s.tickCount%2 == 0 {
				tempChange = 1
			} else {
				tempChange = 0
			}
		case "low":
			// 低速时每三个tick变化0.1度
			if s.tickCount%3 == 0 {
				tempChange = 1
			} else {
				tempChange = 0
			}
		default:
			tempChange = 0
		}

		// 根据制冷或制热模式调整温度变化方向
		if tempChange > 0 {
			if scheduler.Mode == "cooling" {
				// 制冷模式：温度下降
				if scheduler.CurrentTemp > scheduler.TargetTemp {
					scheduler.CurrentTemp -= tempChange
					// 确保不低于目标温度
					if scheduler.CurrentTemp < scheduler.TargetTemp {
						scheduler.CurrentTemp = scheduler.TargetTemp
					}
				}
			} else if scheduler.Mode == "heating" {
				// 制热模式：温度上升
				if scheduler.CurrentTemp < scheduler.TargetTemp {
					scheduler.CurrentTemp += tempChange
					// 确保不高于目标温度
					if scheduler.CurrentTemp > scheduler.TargetTemp {
						scheduler.CurrentTemp = scheduler.TargetTemp
					}
				}
			}

			// 空调花费的金额等于温度变化量
			scheduler.CurrentCost += tempChange
			scheduler.TotalCost += tempChange

		}
		// 增加运行时间
		scheduler.CurrentRunningTime += 6
		scheduler.RunningTime += 6 // 每个tick为6秒

		log.Printf("空调ID %d: 温度 %d°C -> %d°C, 变化量 %d°C, 当前费用 %d, 总费用 %d",
			scheduler.ACID, scheduler.CurrentTemp-tempChange, scheduler.CurrentTemp,
			tempChange, scheduler.CurrentCost, scheduler.TotalCost)
		// 检查当前温度是否等于目标温度，如果是则修改ACState为3（达到目标温度回温）
		if scheduler.CurrentTemp == scheduler.TargetTemp {
			scheduler.ACState = 3
			log.Printf("空调ID %d 已达到目标温度，状态设置为3（达到目标温度回温）", scheduler.ACID)
		}
	}

	log.Printf("完成服务队列温度刷新，队列长度: %d", len(s.servingQueue))
}

// incrementTimeSliceCount 对队列前三中每个进行时间片调度的空调的时间片数加1
func (s *ACScheduler) incrementTimeSliceCount() {
	if s.currentPriority == 0 {
		return // 没有当前时间片调度优先级，不进行时间片计数
	}

	// 对服务队列前三个空调中属于当前调度优先级的空调时间片数加1
	for i := 0; i < len(s.servingQueue) && i < 3; i++ {
		if s.servingQueue[i].Priority == s.currentPriority {
			s.servingQueue[i].RoundRobinCount++
			log.Printf("空调ID %d 时间片数增加到: %d", s.servingQueue[i].ACID, s.servingQueue[i].RoundRobinCount)
		}
	}
}

// checkAndSwapTimeSliceACs 检查队列前三中是否有时间片数为2的，如果有则进行队列交换
func (s *ACScheduler) checkAndSwapTimeSliceACs() {
	if s.currentPriority == 0 || len(s.bufferQueue) == 0 {
		return // 没有当前时间片调度优先级或缓冲队列为空，不进行交换
	}

	// 检查服务队列前三个空调中是否有时间片数为2的
	for i := 0; i < len(s.servingQueue) && i < 3; i++ {
		if s.servingQueue[i].Priority == s.currentPriority && s.servingQueue[i].RoundRobinCount == 2 {
			// 找到缓冲队列中当前优先级的末尾位置
			lastSamePriorityIndex := s.findLastSamePriorityIndex(s.currentPriority)
			if lastSamePriorityIndex != -1 {
				// 将时间片数为2的空调交换至缓冲队列当前优先级末尾
				swapAC := s.servingQueue[i]
				replaceAC := s.bufferQueue[lastSamePriorityIndex]

				// 执行交换
				s.servingQueue[i] = replaceAC
				s.bufferQueue[lastSamePriorityIndex] = swapAC

				// 将新进入服务队列第三位置的空调时间片数改为0
				if i == 2 {
					s.servingQueue[i].RoundRobinCount = 0
				}

				log.Printf("交换空调: 服务队列位置%d的空调ID %d (时间片数2) 与缓冲队列位置%d的空调ID %d",
					i, swapAC.ACID, lastSamePriorityIndex, replaceAC.ACID)
			}
		}
	}
}

// findLastSamePriorityIndex 找到缓冲队列中指定优先级的最后一个位置
func (s *ACScheduler) findLastSamePriorityIndex(priority int) int {
	lastIndex := -1
	for i, scheduler := range s.bufferQueue {
		if scheduler.Priority == priority {
			lastIndex = i
		}
	}
	return lastIndex
}

// sortBufferQueue 对缓冲队列进行排序
func (s *ACScheduler) sortBufferQueue() {
	log.Printf("对缓冲队列进行排序，当前缓冲队列长度: %d", len(s.bufferQueue))

	// 首先按照风速优先级排序（高速1最高，中速2，低速3最低）
	sort.Slice(s.bufferQueue, func(i, j int) bool {
		return s.bufferQueue[i].Priority < s.bufferQueue[j].Priority
	})

	// 如果当前缓冲队列小于等于3，结束排序，清空当前时间片调度优先级，清空队列所有空调时间片数
	if len(s.bufferQueue) <= 3 {
		s.currentPriority = 0
		for _, scheduler := range s.bufferQueue {
			scheduler.RoundRobinCount = 0
		}
		log.Printf("缓冲队列长度<=3，清空时间片调度优先级和时间片数")
		return
	}

	// 如果当前队列排第三的空调优先级高于排第四的空调优先级，结束排序
	if s.bufferQueue[2].Priority < s.bufferQueue[3].Priority {
		s.currentPriority = 0
		for _, scheduler := range s.bufferQueue {
			scheduler.RoundRobinCount = 0
		}
		log.Printf("第三位优先级高于第四位，清空时间片调度优先级和时间片数")
		return
	}

	// 如果当前队列排第三的空调优先级等于排第四的空调优先级
	if s.bufferQueue[2].Priority == s.bufferQueue[3].Priority && s.bufferQueue[2].Priority != s.currentPriority {
		thirdPriority := s.bufferQueue[2].Priority

		// 如果当前时间片调度优先级为空，记录该优先级为当前时间片调度优先级
		if s.currentPriority == 0 {
			s.currentPriority = thirdPriority
			log.Printf("设置当前时间片调度优先级为: %d", s.currentPriority)

		} else if s.currentPriority != thirdPriority {
			// 如果当前调度优先级与该优先级不同，则清空当前时间片调度优先级，清空队列所有空调时间片数
			s.currentPriority = 0
			for _, scheduler := range s.bufferQueue {
				scheduler.RoundRobinCount = 0
			}
			log.Printf("优先级不匹配，清空时间片调度优先级和时间片数")
			return
		}

		// 对该优先级的所有空调根据当前服务时间进行排序
		s.sortByServiceTimeAndID(thirdPriority)

		// 对重新排序后不在队列前三的空调，将其时间片数设置为2，在前三的将其时间片设置为0
		for i, scheduler := range s.bufferQueue {
			if scheduler.Priority == thirdPriority {
				if i < 3 {
					scheduler.RoundRobinCount = 0
				} else {
					scheduler.RoundRobinCount = 2
				}
			}
		}
		log.Printf("完成优先级%d的时间片数设置", thirdPriority)
	}

	// 在每次时间片调度开启时，对队列前三中每个进行时间片调度的空调的时间片数加1
	s.incrementTimeSliceCount()

	// 在完成时间片数增加后，检查队列前三中是否有时间片数为2的，如果有则进行队列交换
	s.checkAndSwapTimeSliceACs()

}

// sortByServiceTimeAndID 对指定优先级的空调按服务时间和ID排序
func (s *ACScheduler) sortByServiceTimeAndID(priority int) {
	// 找出所有指定优先级的空调
	var samepriorityACs []*models.Scheduler
	var otherACs []*models.Scheduler

	for _, scheduler := range s.bufferQueue {
		if scheduler.Priority == priority {
			samepriorityACs = append(samepriorityACs, scheduler)
		} else {
			otherACs = append(otherACs, scheduler)
		}
	}

	// 对同优先级的空调按服务时间排序，如果服务时间相同则按ID排序
	sort.Slice(samepriorityACs, func(i, j int) bool {
		if samepriorityACs[i].RunningTime == samepriorityACs[j].RunningTime {
			return samepriorityACs[i].ACID < samepriorityACs[j].ACID
		}
		return samepriorityACs[i].RunningTime < samepriorityACs[j].RunningTime
	})

	// 重新构建缓冲队列：先放其他优先级的，再放排序后的同优先级的
	s.bufferQueue = make([]*models.Scheduler, 0)
	s.bufferQueue = append(s.bufferQueue, otherACs...)
	s.bufferQueue = append(s.bufferQueue, samepriorityACs...)

	// 重新按优先级排序整个队列
	sort.Slice(s.bufferQueue, func(i, j int) bool {
		return s.bufferQueue[i].Priority < s.bufferQueue[j].Priority
	})

	log.Printf("完成优先级%d的服务时间和ID排序", priority)
}

// updateServingQueue 更新服务队列为缓冲队列排序后的前三个
func (s *ACScheduler) updateServingQueue() {
	// 清空当前服务队列
	s.servingQueue = make([]*models.Scheduler, 0)

	if len(s.bufferQueue) == 0 {
		return
	}

	// 取缓冲队列前三个作为新的服务队列
	maxServing := 3
	if len(s.bufferQueue) < maxServing {
		maxServing = len(s.bufferQueue)
	}

	// 将缓冲队列前三个移到服务队列
	for i := 0; i < maxServing; i++ {
		s.servingQueue = append(s.servingQueue, s.bufferQueue[i])
	}

	// 更新ACState：缓冲队列中不在服务队列的设为1，在服务队列的设为0
	for i, scheduler := range s.bufferQueue {
		if i < maxServing {
			// 在服务队列中的设为0（运行）
			scheduler.ACState = 0
		} else {
			// 不在服务队列中的设为1（在等待序列）
			scheduler.ACState = 1
		}
	}

	log.Printf("服务队列已更新")
}

func (s *ACScheduler) refreshWarmingQueue() {
	// 回温队列中所有空调每2个tick刷新1度
	for _, scheduler := range s.warmingQueue {
		// 每2个tick变化1度
		if s.tickCount%2 == 0 {
			// 根据模式决定回温方向
			if scheduler.Mode == "cool" {
				// 制冷模式回温：温度上升，但不能超过环境温度
				if scheduler.CurrentTemp < scheduler.EnvironmentTemp {
					scheduler.CurrentTemp += 1
					log.Printf("空调ID %d 制冷回温：温度 %d°C -> %d°C", scheduler.ACID, scheduler.CurrentTemp-1, scheduler.CurrentTemp)
				} else {
					log.Printf("空调ID %d 制冷回温：已达到环境温度 %d°C，停止回温", scheduler.ACID, scheduler.EnvironmentTemp)
				}
			} else if scheduler.Mode == "heat" {
				// 制热模式回温：温度下降，但不能低于环境温度
				if scheduler.CurrentTemp > scheduler.EnvironmentTemp {
					scheduler.CurrentTemp -= 1
					log.Printf("空调ID %d 制热回温：温度 %d°C -> %d°C", scheduler.ACID, scheduler.CurrentTemp+1, scheduler.CurrentTemp)
				} else {
					log.Printf("空调ID %d 制热回温：已达到环境温度 %d°C，停止回温", scheduler.ACID, scheduler.EnvironmentTemp)
				}
			}
		}
	}

	if len(s.warmingQueue) > 0 {
		log.Printf("回温队列已更新，当前队列长度: %d", len(s.warmingQueue))
	}
}

// updateWarmingQueue 更新回温队列
func (s *ACScheduler) updateWarmingQueue() {
	// 第一步：检查缓冲队列中ACState为2或3的，将其移除并加入回温队列
	var newBufferQueue []*models.Scheduler
	for _, scheduler := range s.bufferQueue {
		if scheduler.ACState == 2 || scheduler.ACState == 3 {
			// 当ACState为2时，需要额外操作：在空调操作表中查找当前账单号最后一次关机调度并保存信息
			if scheduler.ACState == 2 {
				s.saveShutdownOperationToDB(scheduler)
			}

			// 移除并加入回温队列
			s.warmingQueue = append(s.warmingQueue, scheduler)
			log.Printf("空调ID %d 从缓冲队列移除并加入回温队列，ACState: %d", scheduler.ACID, scheduler.ACState)
		} else {
			// 保留在缓冲队列中
			newBufferQueue = append(newBufferQueue, scheduler)
		}
	}
	s.bufferQueue = newBufferQueue

	// 第二步：检查回温队列中ACState为3且当前温度和目标温度差值绝对值为10的
	var newWarmingQueue []*models.Scheduler
	for _, scheduler := range s.warmingQueue {
		if scheduler.ACState == 3 {
			// 计算温度差值的绝对值
			tempDiff := scheduler.CurrentTemp - scheduler.TargetTemp
			if tempDiff < 0 {
				tempDiff = -tempDiff
			}

			if tempDiff == 10 {
				// 修改ACState为1，移出回温队列，加入缓冲队列
				scheduler.ACState = 1
				s.bufferQueue = append(s.bufferQueue, scheduler)
				log.Printf("空调ID %d 从回温队列移除并加入缓冲队列，ACState设置为1（在等待序列），温度差值: %d", scheduler.ACID, tempDiff)
			} else {
				// 保留在回温队列中
				newWarmingQueue = append(newWarmingQueue, scheduler)
			}
		} else {
			// 保留在回温队列中
			newWarmingQueue = append(newWarmingQueue, scheduler)
		}
	}
	s.warmingQueue = newWarmingQueue

	log.Printf("updateWarmingQueue完成 - 缓冲队列: %d, 回温队列: %d", len(s.bufferQueue), len(s.warmingQueue))
}

// GetSchedulerStatus 获取调度器状态（管理员接口）
func GetSchedulerStatus(c *gin.Context) {
	scheduler := GetScheduler()
	scheduler.mu.RLock()
	defer scheduler.mu.RUnlock()

	c.JSON(200, gin.H{
		"message":          "获取调度器状态成功",
		"is_running":       scheduler.isRunning,
		"serving_count":    len(scheduler.servingQueue),
		"buffer_count":     len(scheduler.bufferQueue),
		"warming_count":    len(scheduler.warmingQueue),
		"total_requests":   len(scheduler.schedulers),
		"tick_count":       scheduler.tickCount,
		"current_priority": scheduler.currentPriority,
		"first_ac_added":   scheduler.firstACAdded,
		"serving_queue":    scheduler.servingQueue,
		"buffer_queue":     scheduler.bufferQueue,
		"warming_queue":    scheduler.warmingQueue,
	})
}

// saveACStatesToDB 保存空调状态到数据库
// 按照指定顺序：先保存服务队列中的内容，再保存缓存队列第四个开始的内容，最后保存回温队列中的内容
func (s *ACScheduler) saveACStatesToDB() {
	// 1. 先保存服务队列中的内容
	for _, ac := range s.servingQueue {
		s.saveACDetailToDB(ac, 0) // ACStatus = 0 表示运行状态
	}

	// 2. 再保存缓存队列第四个开始的内容
	for i := 3; i < len(s.bufferQueue); i++ { // 从第4个开始（索引3）
		s.saveACDetailToDB(s.bufferQueue[i], 1) // ACStatus = 1 表示在等待序列
	}

	// 3. 最后保存回温队列中的内容
	for _, ac := range s.warmingQueue {
		// 根据ACState判断回温状态：2-关机回温，3-达到目标温度回温
		acStatus := 2
		if ac.ACState == 3 {
			acStatus = 3
		}
		s.saveACDetailToDB(ac, acStatus)
	}

	log.Printf("已保存空调状态到数据库 - 服务队列: %d, 缓冲队列(第4个开始): %d, 回温队列: %d",
		len(s.servingQueue),
		max(0, len(s.bufferQueue)-3),
		len(s.warmingQueue))
}

// saveACDetailToDB 保存单个空调状态到数据库
func (s *ACScheduler) saveACDetailToDB(ac *models.Scheduler, acStatus int) {
	// 计算费率（根据风速）
	var rate float32
	switch ac.CurrentSpeed {
	case "high":
		rate = 1.0
	case "medium":
		rate = 0.5
	case "low":
		rate = 0.33
	default:
		rate = 0.5
	}

	// 计算温度变化（当前温度与环境温度的差值）
	tempChange := ac.CurrentTemp - ac.EnvironmentTemp

	// 创建空调状态记录
	acDetail := models.AirConditionerDetail{
		BillID:             ac.BillID,
		RoomID:             ac.RoomID,
		AcID:               ac.ACID,
		ACStatus:           acStatus,
		Speed:              ac.CurrentSpeed,
		Mode:               ac.Mode,
		TargetTemp:         ac.TargetTemp,
		EnvironmentTemp:    ac.EnvironmentTemp,
		CurrentTemp:        ac.CurrentTemp,
		RunningTime:        ac.RunningTime,
		CurrentRunningTime: ac.CurrentRunningTime,
		CurrentCost:        float32(ac.CurrentCost),
		TotalCost:          float32(ac.TotalCost),
		Rate:               rate,
		TempChange:         tempChange,
	}

	// 保存到数据库
	if err := database.DB.Create(&acDetail).Error; err != nil {
		log.Printf("保存空调状态失败 - RoomID: %d, ACID: %d, Error: %v", ac.RoomID, ac.ACID, err)
	} else {
		log.Printf("保存空调状态成功 - RoomID: %d, ACID: %d, Status: %d", ac.RoomID, ac.ACID, acStatus)
	}
}

// saveShutdownOperationToDB 当ACState为2时，在空调操作表中查找当前账单号最后一次关机调度并保存信息
func (s *ACScheduler) saveShutdownOperationToDB(scheduler *models.Scheduler) {
	// 查找当前账单号最后一次关机调度（OperationState = 1表示关机）
	var lastShutdownOp models.AirConditionerOperation
	err := database.DB.Where("bill_id = ? AND room_id = ? AND operation_state = ?",
		scheduler.BillID, scheduler.RoomID, 1).Order("created_at DESC").First(&lastShutdownOp).Error

	if err != nil {
		log.Printf("未找到账单号 %d 房间 %d 的最后一次关机调度记录: %v", scheduler.BillID, scheduler.RoomID, err)
		return
	}

	// 更新最后一次关机调度记录，保存当前花费、当前温度、当前时间
	lastShutdownOp.CurrentCost = float32(scheduler.CurrentCost)
	lastShutdownOp.CurrentTemp = scheduler.CurrentTemp
	lastShutdownOp.RunningTime = scheduler.RunningTime // 使用RunningTime作为当前时间
	lastShutdownOp.CurrentRunningTime = scheduler.CurrentRunningTime
	lastShutdownOp.UpdatedAt = time.Now()

	// 保存更新到数据库
	if err := database.DB.Save(&lastShutdownOp).Error; err != nil {
		log.Printf("保存关机调度信息失败 - BillID: %d, RoomID: %d, Error: %v",
			scheduler.BillID, scheduler.RoomID, err)
	} else {
		log.Printf("保存关机调度信息成功 - BillID: %d, RoomID: %d, CurrentCost: %.2f, CurrentTemp: %d, RunningTime: %d",
			scheduler.BillID, scheduler.RoomID, float32(scheduler.CurrentCost), scheduler.CurrentTemp, scheduler.RunningTime)
	}
}

// max 返回两个整数中的较大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
