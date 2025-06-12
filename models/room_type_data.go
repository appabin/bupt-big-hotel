package models

import "github.com/lib/pq"

// GetDefaultRoomTypes 返回默认的房间类型数据
func GetDefaultRoomTypes() []RoomType {
	return []RoomType{
		{
			Type:        "测试房间",
			Description: "测试制热空调使用",
			PriceRange:  "100-200元",
			Features:    pq.StringArray{"单人床", "制热测试", "空调"},
		},
		{
			Type:        "单人间",
			Description: "适合单人住宿，经济实惠",
			PriceRange:  "150-280元",
			Features:    pq.StringArray{"单人床", "24小时热水", "免费WiFi", "空调"},
		},
		{
			Type:        "双人间",
			Description: "适合情侣或朋友住宿",
			PriceRange:  "280-380元",
			Features:    pq.StringArray{"双人床", "24小时热水", "免费WiFi", "空调", "迷你吧"},
		},
		{
			Type:        "标准间",
			Description: "商务人士首选，设施齐全",
			PriceRange:  "380-480元",
			Features:    pq.StringArray{"大床", "工作台", "免费WiFi", "空调", "保险箱", "浴缸"},
		},
		{
			Type:        "豪华间",
			Description: "豪华装修，享受优质服务",
			PriceRange:  "480-680元",
			Features:    pq.StringArray{"特大床", "豪华浴室", "免费WiFi", "中央空调", "迷你吧", "24小时客房服务"},
		},
	}
}
