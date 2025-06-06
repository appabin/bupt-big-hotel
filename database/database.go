package database

import (
	"bupt-hotel/models"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitDatabase 初始化数据库连接
func InitDatabase(databasePath string) error {
	var err error
	DB, err = gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		return err
	}

	// 自动迁移数据库表结构
	err = DB.AutoMigrate(
		&models.User{},
		&models.RoomType{},
		&models.RoomInfo{},
		&models.AirConditioner{},
		&models.Detail{},
		&models.RoomOperation{},
	)
	if err != nil {
		return err
	}

	// 初始化基础数据
	initializeData()

	log.Println("数据库初始化完成")
	return nil
}

// initializeData 初始化基础数据
func initializeData() {
	// 初始化房间类型数据
	var roomTypeCount int64
	DB.Model(&models.RoomType{}).Count(&roomTypeCount)
	if roomTypeCount == 0 {
		roomTypes := models.GetDefaultRoomTypes()
		for _, roomType := range roomTypes {
			DB.Create(&roomType)
		}
		log.Println("初始化房间类型数据完成")
	}

	// 检查是否已有房间数据，如果没有则创建示例房间
	var roomCount int64
	DB.Model(&models.RoomInfo{}).Count(&roomCount)
	if roomCount == 0 {
		// 创建10个示例房间，分配不同的房间类型
		for j := 1; j <= 4; j++ {
			for i := 1; i <= 10; i++ {
				// 循环分配房间类型 (1-4)
				roomTypeID := j
				room := models.RoomInfo{
					RoomID:     j*100 + i,
					RoomTypeID: roomTypeID,
					State:      0, // 空房
					DailyRate:  float32(80.00 + j*100),
					Deposit:    500.00,
				}
				DB.Create(&room)

				// 为每个房间创建对应的空调
				ac := models.AirConditioner{
					RoomID:          j*100 + i,
					ACState:         0, // 关闭
					Mode:            "cooling",
					CurrentSpeed:    "low",
					CurrentTemp:     250,
					TargetTemp:      250,
					InitialTemp:     250,
					LastPowerOnTime: time.Now(),
					SwitchCount:     0,
				}
				DB.Create(&ac)
			}
			log.Println("初始化房间和空调数据完成")
		}

	}

	// 检查是否已有管理员账户
	var adminCount int64
	DB.Model(&models.User{}).Where("identity = ?", "administrator").Count(&adminCount)
	if adminCount == 0 {
		// 创建默认管理员账户
		admin := models.User{
			Username: "admin",
			Password: "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi", // password
			Identity: "administrator",
		}
		DB.Create(&admin)
		log.Println("创建默认管理员账户: admin/password")
	}
}
