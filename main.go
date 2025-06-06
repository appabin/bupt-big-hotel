package main

import (
	"bupt-hotel/database"
	"bupt-hotel/handlers"
	"bupt-hotel/middleware"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	config := LoadConfig()

	// 初始化JWT
	middleware.InitJWT(config.JWTSecret)

	// 初始化数据库
	if err := database.InitDatabase(config.DatabasePath); err != nil {
		log.Fatal("数据库初始化失败:", err)
	}

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 创建Gin路由器
	r := gin.Default()

	// 添加CORS中间件
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "BUPT酒店管理系统运行正常",
		})
	})

	// API路由组
	api := r.Group("/api")
	{
		// 公开路由（无需认证）
		public := api.Group("/public")
		{
			public.POST("/register", handlers.Register) // 用户注册
			public.POST("/login", handlers.Login)       // 用户登录
		}

		// 需要认证的路由
		auth := api.Group("/auth")
		auth.Use(middleware.AuthMiddleware())
		{
			// 房间相关路由
			rooms := auth.Group("/rooms")
			{
				rooms.GET("/by-type/:type_id", handlers.GetRoomsByType) // 获取房间信息
				rooms.GET("/type", handlers.GetAllRoomTypes)            // 获取所有房间
				rooms.GET("/available", handlers.GetAvailableRooms)     // 获取空房间
				rooms.GET("/my", handlers.GetMyRooms)                   // 获取我的房间
				rooms.POST("/book", handlers.BookRoom)                  // 订房
				rooms.POST("/:room_id/checkout", handlers.CheckoutRoom) // 退房
			}

			// 空调相关路由
			ac := auth.Group("/airconditioner")
			{
				ac.GET("/:room_id", handlers.GetAirConditioner)             // 获取房间空调信息
				ac.PUT("/:room_id", handlers.ControlAirConditioner)         // 控制空调
				ac.GET("/:room_id/status", handlers.GetACStatusLongPolling) // 长轮询获取空调状态
			}
		}

		// 管理员路由
		admin := api.Group("/admin")
		admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
		{
			admin.GET("/rooms", handlers.GetAllRooms)                     // 获取所有房间
			admin.GET("/airconditioners", handlers.GetAllAirConditioners) // 获取所有空调信息
			admin.GET("/scheduler/status", handlers.GetSchedulerStatus)   // 获取调度器状态
			admin.PUT("/room-types/:id", handlers.UpdateRoomType)         // 修改指定ID的房间类型
		}
	}

	// 启动服务器
	log.Printf("服务器启动在端口 %s", config.ServerPort)
	log.Printf("健康检查: http://localhost%s/health", config.ServerPort)
	log.Printf("API文档:")
	log.Printf("  POST /api/public/register - 用户注册")
	log.Printf("  POST /api/public/login - 用户登录")
	log.Printf("  GET  /api/auth/rooms/available - 获取空房间")
	log.Printf("  GET  /api/auth/rooms/my - 获取我的房间")
	log.Printf("  POST /api/auth/rooms/book - 订房")
	log.Printf("  DELETE /api/auth/rooms/:room_id/checkout - 退房")
	log.Printf("  GET  /api/auth/airconditioner/:room_id - 获取空调信息")
	log.Printf("  PUT  /api/auth/airconditioner/:room_id - 控制空调")
	log.Printf("  GET  /api/auth/airconditioner/:room_id/status - 长轮询获取空调状态")
	log.Printf("  GET  /api/admin/rooms - 获取所有房间(管理员)")
	log.Printf("  GET  /api/admin/airconditioners - 获取所有空调(管理员)")
	log.Printf("  GET  /api/admin/scheduler/status - 获取空调调度器状态(管理员)")

	if err := r.Run(config.ServerPort); err != nil {
		log.Fatal("服务器启动失败:", err)
	}
}
