package main

import (
	"aiplatform/common/mysql"
	"aiplatform/common/rabbitmq"
	"aiplatform/common/redis"
	"aiplatform/config"
	"aiplatform/router"
	"fmt"
	"log"
)

func StartServer(addr string, port int) error {
	r := router.InitRouter()
	return r.Run(fmt.Sprintf("%s:%d", addr, port))
}

func main() {
	conf := config.GetConfig()
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	if err := mysql.InitMysql(); err != nil {
		log.Println("InitMysql error , " + err.Error())
		return
	}

	redis.Init()
	log.Println("redis init success")

	rabbitmq.InitRabbitMQ()
	log.Println("rabbitmq init success")

	if err := StartServer(host, port); err != nil {
		panic(err)
	}
}
