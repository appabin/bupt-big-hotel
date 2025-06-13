package main

import (
	"log"
	"os"
)

type Config struct {
	DatabasePath string
	JWTSecret    string
	ServerPort   string
}

func LoadConfig() *Config {
	// 从环境变量获取配置，如果没有则使用默认值
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "bupt-hotel-secret-key-2025" // 默认密钥，生产环境应使用环境变量
	}

	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "./hotel.db" // 默认数据库路径
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = ":8099" // 默认端口
	}

	log.Printf("配置加载完成: 数据库路径=%s, 服务端口=%s", databasePath, serverPort)

	return &Config{
		DatabasePath: databasePath,
		JWTSecret:    jwtSecret,
		ServerPort:   serverPort,
	}
}
