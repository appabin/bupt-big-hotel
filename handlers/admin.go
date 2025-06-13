package handlers

import (
	"bupt-hotel/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetAdminSchedulerStatus 获取调度器状态（管理员专用接口）
// 返回运行队列、缓存队列第4个空调后的部分、回温队列的详细信息
func GetAdminSchedulerStatus(c *gin.Context) {
	scheduler := GetScheduler()
	scheduler.mu.RLock()
	defer scheduler.mu.RUnlock()

	// 获取缓存队列第4个空调后的部分
	var bufferQueueAfterFourth []*models.Scheduler
	if len(scheduler.bufferQueue) > 3 {
		bufferQueueAfterFourth = scheduler.bufferQueue[3:]
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "获取管理员调度器状态成功",
		"data": gin.H{
			"scheduler_status": gin.H{
				"is_running":       scheduler.isRunning,
				"tick_count":       scheduler.tickCount,
				"current_priority": scheduler.currentPriority,
				"total_requests":   len(scheduler.schedulers),
			},
			"queues": gin.H{
				"serving_queue": gin.H{
					"count": len(scheduler.servingQueue),
					"items": scheduler.servingQueue,
				},
				"buffer_queue_after_fourth": gin.H{
					"count": len(bufferQueueAfterFourth),
					"items": bufferQueueAfterFourth,
				},
				"warming_queue": gin.H{
					"count": len(scheduler.warmingQueue),
					"items": scheduler.warmingQueue,
				},
				"buffer_queue_total_count": len(scheduler.bufferQueue),
			},
		},
	})
}

