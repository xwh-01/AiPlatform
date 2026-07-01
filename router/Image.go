package router

import (
	"aiplatform/controller/image"

	"github.com/gin-gonic/gin"
)

func ImageRouter(r *gin.RouterGroup) {

	r.POST("/recognize", image.RecognizeImage)
}
