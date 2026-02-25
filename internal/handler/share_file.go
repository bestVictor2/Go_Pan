package handler

import (
	"CloudVault/internal/activity"
	"CloudVault/internal/dto"
	"CloudVault/internal/service"
	"CloudVault/utils"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

// CreateShareHandler creates a share link.
func CreateShareHandler(c *gin.Context) {
	var req dto.CreateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"msg": "invalid params"})
		return
	}
	value, _ := c.Get("user_id")
	userID, _ := value.(uint64)
	share, err := service.CreateShare(userID, req.FileID, req.ExpireDays, req.NeedCode)
	if err != nil {
		c.JSON(500, gin.H{"msg": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"share_id":     share.ShareID,
		"extract_code": share.ExtractCode,
	})
}

// ShareDownload downloads a shared file.
/*
分享链接是否正确？
提取码是否正确？
文件是否安全存在？
如何进行高效安全的下载？
记录该文件被谁下载过
*/
func ShareDownload(c *gin.Context) {
	shareID := c.Param("shareID")
	extractCode := strings.TrimSpace(c.Query("extract_code"))
	if extractCode == "" {
		extractCode = strings.TrimSpace(c.PostForm("extract_code"))
	}

	share, err := service.CheckShare(shareID, extractCode)
	if err != nil {
		c.JSON(403, gin.H{"msg": err.Error()})
		return
	}

	userFile, err := service.GetUserFileById(share.FileID)
	if err != nil {
		c.JSON(404, gin.H{"msg": "file not found"})
		return
	}

	if userFile.ObjectID == nil {
		c.JSON(404, gin.H{"msg": "file not found"})
		return
	}
	fileObject, err := service.GetFileObjectById(*userFile.ObjectID)
	if err != nil {
		c.JSON(404, gin.H{"msg": "file not found"})
		return
	}

	object, info, err := service.MinioDownloadObject(
		c.Request.Context(),
		fileObject.ObjectName,
	)
	if err != nil {
		c.JSON(500, gin.H{"msg": err.Error()})
		return
	}
	defer object.Close()

	// 设置响应头阶段
	safeName := utils.SanitizeHeaderFilename(userFile.Name)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
	contentType := service.GetContentBook(userFile.Name)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", info.Size))

	if _, err := io.Copy(c.Writer, object); err != nil {
		log.Printf("share download stream failed: %v", err)
		return
	}
	_ = service.LogShareAccess(share, service.ShareAccessMeta{ // 记录分享访问日志
		VisitorIP: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Referer:   c.Request.Referer(),
	})
	_ = activity.Emit(c.Request.Context(), share.UserID, activity.ActionDownload, share.FileID, info.Size) //记录下载行为埋点
}
