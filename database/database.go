package database

import (
	"bupt-hotel/models"
	"log"

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
		&models.AirConditionerDetail{},
		&models.RoomOperation{},
		&models.AirConditionerOperation{},
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

		var TestRoomTemp = []int{100, 150, 180, 120, 140}
		var TestRoomDailyRate = []int{100, 125, 150, 200, 100}
		for i := 1; i <= 5; i++ {
			room := models.RoomInfo{
				RoomID:     100 + i,
				RoomTypeID: 1,
				State:      0, // 空房
				DailyRate:  float32(TestRoomDailyRate[i-1]),
				Deposit:    500.00,
			}
			DB.Create(&room)
			ac := models.AirConditioner{
				ID:              100 + i,
				RoomID:          100 + i,
				EnvironmentTemp: TestRoomTemp[i-1],
			}
			DB.Create(&ac)
		}

		for j := 2; j <= 5; j++ {
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
					ID:              j*100 + i,
					EnvironmentTemp: 150,
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
