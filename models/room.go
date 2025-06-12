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

// 房间账单记录表
type RoomOperation struct {
	ID            int       `gorm:"primaryKey"`
	RoomID        int       `gorm:"type:int;index"`
	BillID        int       `gorm:"type:int;index"` // 账单号，与空调操作表保持一致
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
