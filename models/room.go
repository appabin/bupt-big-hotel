package models

import (
	"time"

	"github.com/lib/pq"
)

// 房间类型表
type RoomType struct {
	ID          int            `gorm:"primaryKey;autoIncrement"`
	Type        string         `gorm:"type:varchar(50);not null;unique"` // 房间类型名称
	Description string         `gorm:"type:text"`                        // 房间描述
	PriceRange  string         `gorm:"type:varchar(100)"`                // 价格范围
	Features    pq.StringArray `gorm:"type:text[]"`                      // 房间特色功能列表
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
}

// 房间信息表
type RoomInfo struct {
	RoomID       int       `gorm:"primaryKey"`
	RoomTypeID   int       `gorm:"type:int;index"`        // 关联房间类型ID
	RoomType     RoomType  `gorm:"foreignKey:RoomTypeID"` // 房间类型关联
	ClientID     string    `gorm:"type:varchar(255)"`
	ClientName   string    `gorm:"type:varchar(255)"`
	CheckinTime  time.Time `gorm:"type:datetime"`
	CheckoutTime time.Time `gorm:"type:datetime"`
	State        int       // 0: 空房 1: 已入住
	DailyRate    float32   `gorm:"type:float(7,2)"`  // 每日房费
	Deposit      float32   `gorm:"type:float(10,2)"` // 押金
}

// 空调信息表
type AirConditioner struct {
	ID              int       `gorm:"primaryKey"`
	RoomID          int       `gorm:"type:int;index"`   // 关联房间ID
	ACState         int       `gorm:"type:int"`         // 0: 关闭 1: 开启
	Mode            string    `gorm:"type:varchar(20)"` // cooling/heating
	CurrentSpeed    string    `gorm:"type:varchar(255)"`
	CurrentTemp     int       `gorm:"type:int"`      // 当前温度*10，如245表示24.5度
	TargetTemp      int       `gorm:"type:int"`      // 目标温度*10，如245表示24.5度
	InitialTemp     int       `gorm:"type:int"`      // 初始温度*10，如245表示24.5度
	LastPowerOnTime time.Time `gorm:"type:datetime"` // 记录最后一次开机时间
	SwitchCount     int       `gorm:"type:int;default:0"`
}

// 空调操作详情表
type Detail struct {
	ID          int       `gorm:"primary_key"`
	RoomID      int       `gorm:"type:int"`
	QueryTime   time.Time `gorm:"type:datetime"`
	StartTime   time.Time `gorm:"type:datetime"`
	EndTime     time.Time `gorm:"type:datetime"`
	ServeTime   float32   `gorm:"type:float(7,2)"` // 服务时长(分钟)
	Speed       string    `gorm:"type:varchar(255)"`
	Mode        string    `gorm:"type:varchar(255)"` // 空调模式：cooling/heating
	Cost        float32   `gorm:"type:float(7,2)"`   // 费用(元)
	Rate        float32   `gorm:"type:float(5,2)"`   // 每分钟费率(元/分钟)
	TempChange  int       `gorm:"type:int"`          // 温度变化*10
	CurrentTemp int       `gorm:"type:int"`          // 当前温度*10
	TargetTemp  int       `gorm:"type:int"`          // 目标温度*10
}

// 房间操作记录表
type RoomOperation struct {
	ID            int       `gorm:"primaryKey"`
	RoomID        int       `gorm:"type:int;index"`
	ClientID      string    `gorm:"type:varchar(255)"`
	ClientName    string    `gorm:"type:varchar(255)"`
	OperationType string    `gorm:"type:varchar(50)"` // checkin, checkout
	OperationTime time.Time `gorm:"type:datetime"`
	CheckinTime   time.Time `gorm:"type:datetime"`
	CheckoutTime  time.Time `gorm:"type:datetime"`
	DailyRate     float32   `gorm:"type:float(7,2)"`  // 每日房费
	Deposit       float32   `gorm:"type:float(10,2)"` // 押金
	TotalCost     float32   `gorm:"type:float(10,2)"` // 总费用
	ActualDays    int       `gorm:"type:int"`         // 实际入住天数
}
