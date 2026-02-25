package service

import (
	"CloudVault/config"
	"CloudVault/internal/dto"
	"CloudVault/internal/repo"
	"CloudVault/internal/storage"
	"CloudVault/model"
	"CloudVault/utils"
	"bytes"
	"errors"
	"fmt"
	"time"

	"golang.org/x/net/context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const fileObjectCacheTTL = 5 * time.Minute

func cacheFileObject(ctx context.Context, obj *model.FileObject) {
	if obj == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = utils.SetFileObjectToCache(ctx, obj.ID, obj, fileObjectCacheTTL)
	if obj.Hash != "" {
		_ = utils.SetFileObjectIDByHash(ctx, obj.Hash, obj.ID, fileObjectCacheTTL)
	}
	if obj.BucketName != "" && obj.ObjectName != "" {
		_ = utils.SetFileObjectIDByPath(ctx, obj.BucketName, obj.ObjectName, obj.ID, fileObjectCacheTTL)
	}
}

// BuildObjectName builds object path for a user's hash.
func BuildObjectName(username, hash string) string {
	return fmt.Sprintf("files/%s/%s", username, hash)
} // minio 存储路径

// DeleteMinioFile removes an object from MinIO.
// MinIO 删除对象
func DeleteMinioFile(fileObject *model.FileObject) error {
	ctx := context.Background()
	if storage.Default == nil {
		return fmt.Errorf("storage not initialized")
	}
	return storage.Default.RemoveObject(
		ctx,
		fileObject.BucketName,
		fileObject.ObjectName,
	)
}

// CreateFilesObject inserts a file object record.
func CreateFilesObject(dir *model.FileObject) error {
	if err := repo.Db.Model(&model.FileObject{}).Create(dir).Error; err != nil {
		return err
	}
	cacheFileObject(context.Background(), dir)
	return nil
}

// GetFileByObject finds a file object by bucket and name.
func GetFileByObject(bucket, object string) (*model.FileObject, error) {
	if id, ok := utils.GetFileObjectIDByPath(context.Background(), bucket, object); ok {
		file, err := GetFileObjectById(id)
		if err == nil {
			return file, nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) { // 数据库中不存在但是缓存中存在 处理脏数据
			_ = utils.InvalidateFileObjectPathCache(context.Background(), bucket, object)
		} else {
			return nil, err
		}
	}
	var file model.FileObject
	err := repo.Db.Where(
		"bucket_name = ? AND object_name = ?",
		bucket, object,
	).First(&file).Error
	if err == nil {
		cacheFileObject(context.Background(), &file)
	}
	return &file, err
}

// GetFileObjectByHash finds a file object by hash.
func GetFileObjectByHash(hash string) (*model.FileObject, error) {
	if id, ok := utils.GetFileObjectIDByHash(context.Background(), hash); ok {
		obj, err := GetFileObjectById(id)
		if err == nil {
			return obj, nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = utils.InvalidateFileObjectHashCache(context.Background(), hash)
		} else {
			return nil, err
		}
	}
	var obj model.FileObject
	err := repo.Db.Where("hash = ?", hash).First(&obj).Error
	if err == nil {
		cacheFileObject(context.Background(), &obj)
	}
	return &obj, err
}

// GetFileObjectById finds a file object by ID.
// About Why This Not solve Be id -> cache Not Path -> id -> cache
func GetFileObjectById(id uint64) (*model.FileObject, error) {
	if cached, ok := utils.GetFileObjectFromCache(context.Background(), id); ok && cached != nil {
		cacheFileObject(context.Background(), cached)
		return cached, nil
	}

	var file model.FileObject
	err := repo.Db.Where("id = ?", id).First(&file).Error
	if err == nil {
		cacheFileObject(context.Background(), &file)
	}
	return &file, err
}

// IncreaseRefCount increments object reference count.
func IncreaseRefCount(id uint64) error {
	if err := repo.Db.Model(&model.FileObject{}).
		Where("id = ?", id).
		UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).Error; err != nil { // auto in db
		return err
	}
	_ = utils.InvalidateFileObjectCache(context.Background(), id)
	return nil
}

// DecreaseRefCount decrements object reference count.
func DecreaseRefCount(id uint64) (int, error) {
	var fileObject model.FileObject
	if err := repo.Db.Where("id = ?", id).First(&fileObject).Error; err != nil {
		return 0, err
	}
	if fileObject.RefCount > 1 {
		if err := repo.Db.Model(&model.FileObject{}).
			Where("id = ?", id).
			UpdateColumn("ref_count", gorm.Expr("ref_count - 1")).Error; err != nil {
			return 0, err
		}
		_ = utils.InvalidateFileObjectCache(context.Background(), id)
		return fileObject.RefCount - 1, nil
	}
	return 0, nil
}

// isFileObjectAvailable checks whether the physical object exists in storage.
func isFileObjectAvailable(ctx context.Context, obj *model.FileObject) (bool, error) {
	if obj == nil {
		return false, nil
	}
	if storage.Default == nil {
		return false, fmt.Errorf("storage not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reader, _, err := storage.Default.GetObject(ctx, obj.BucketName, obj.ObjectName)
	if err != nil {
		return false, nil
	}
	_ = reader.Close()
	return true, nil
}

// FastUpload handles hash-based instant upload.
func FastUpload(
	ctx context.Context,
	req *dto.UploadFileByHashRequest,
) (*dto.FastUploadResponse, error) {
	obj, err := GetFileObjectByHash(req.Hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &dto.FastUploadResponse{
				Instant:    false,
				NeedUpload: true,
				Reason:     "hash_not_found",
			}, nil
		}
		return nil, err
	}

	if req.Size > 0 && obj.Size > 0 && req.Size != obj.Size {
		return &dto.FastUploadResponse{
			Instant:    false,
			NeedUpload: true,
			Reason:     "size_mismatch",
		}, nil
	}

	available, err := isFileObjectAvailable(ctx, obj)
	if err != nil {
		return nil, err
	}
	if !available {
		return &dto.FastUploadResponse{
			Instant:    false,
			NeedUpload: true,
			Reason:     "object_missing",
		}, nil
	}

	if err := IncreaseRefCount(obj.ID); err != nil {
		return nil, err
	}
	var parentID *uint64
	if req.ParentId != 0 {
		parentID = &req.ParentId
	}
	userFile := &model.UserFile{
		UserID:   req.UserId,
		ParentID: parentID,
		Name:     req.FileName,
		ObjectID: &obj.ID,
		Size:     obj.Size,
		IsDir:    false,
	}
	if err := CreateUserFileEntry(userFile); err != nil { // 创建文件失败 回滚引用计数
		_, _ = DecreaseRefCount(obj.ID)
		return nil, err
	}
	return &dto.FastUploadResponse{
		Instant: true,
		FileId:  userFile.ID,
	}, nil
}

// GetUploadSessionByHash loads an upload session by hash and user.
func GetUploadSessionByHash(userID uint64, hash string) (*model.UploadSession, error) {
	var session model.UploadSession
	if err := repo.Db.
		Where("file_hash = ? AND user_id = ?", hash, userID).
		Order("id desc").
		First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// GetUploadSessionByUploadID loads an upload session by upload ID.
func GetUploadSessionByUploadID(uploadID string) (*model.UploadSession, error) {
	var session model.UploadSession
	if err := repo.Db.Where("upload_id = ?", uploadID).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// CheckChunkNum loads uploaded chunks for a hash.
func CheckChunkNum(userID uint64, hash string, chunks *[]model.FileChunk) error {
	session, err := GetUploadSessionByHash(userID, hash)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	return repo.Db.
		Where("upload_id = ? AND status = 1", session.UploadID).
		Find(chunks).Error
}

// MultiPartFileInit initializes multipart upload.
func MultiPartFileInit(ctx context.Context, req dto.MultipartInitRequest) (*dto.MultiPartFileResponse, error) {
	if obj, err := GetFileObjectByHash(req.Hash); err == nil { // db
		available, checkErr := isFileObjectAvailable(ctx, obj) // minio
		if checkErr != nil {
			return nil, checkErr
		}
		if !available {
			goto uploadFlow
		}
		if err := IncreaseRefCount(obj.ID); err != nil {
			return nil, err
		}
		var parentID *uint64
		if req.ParentId != 0 {
			parentID = &req.ParentId
		}
		userFile := &model.UserFile{
			UserID:   req.UserId,
			ParentID: parentID,
			Name:     req.FileName,
			ObjectID: &obj.ID,
			Size:     obj.Size,
			IsDir:    false,
		}
		if err := CreateUserFileEntry(userFile); err != nil {
			_, _ = DecreaseRefCount(obj.ID)
			return nil, err
		}
		return &dto.MultiPartFileResponse{
			Instant: true,
		}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) { // 如果不是没有找到记录 也即是产生了其他错误
		return nil, err
	}

uploadFlow: // hash noExist || no fileObject
	chunks := make([]model.FileChunk, 0)
	if err := CheckChunkNum(req.UserId, req.Hash, &chunks); err != nil {
		return nil, err
	}
	uploaded := make([]int, 0, len(chunks))
	for _, c := range chunks {
		uploaded = append(uploaded, c.ChunkIndex)
	}
	if len(uploaded) == 0 {
		if err := CreateUploadSession(req); err != nil {
			return nil, err
		}
	}
	var uploadID string
	var session model.UploadSession
	if err := repo.Db.Where("file_hash = ? AND user_id = ?", req.Hash, req.UserId).Order("id desc").First(&session).Error; err == nil {
		uploadID = session.UploadID
	}
	return &dto.MultiPartFileResponse{
		Instant:  false,
		UploadID: uploadID,
		Uploaded: uploaded,
	}, nil
}

// CreateUploadSession creates an upload session record.
func CreateUploadSession(req dto.MultipartInitRequest) error {
	session := model.UploadSession{
		UploadID:    utils.GetToken(),
		UserID:      req.UserId,
		FileHash:    req.Hash,
		FileName:    req.FileName,
		FileSize:    req.Size,
		ChunkSize:   req.ChunkSize,
		TotalChunks: req.TotalChunks,
		Status:      0,
	}
	return repo.Db.Create(&session).Error
}

// UploadChunk stores a chunk in MinIO and the database.
// MinIO 与数据库
func UploadChunk(
	ctx context.Context,
	req *dto.MultipartUploadChunkRequest,
) error {
	src, err := req.File.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	objectPath := fmt.Sprintf("chunks/%s/%d", req.UploadID, req.ChunkIndex)
	if storage.Default == nil {
		return fmt.Errorf("storage not initialized")
	}
	if err := storage.Default.PutObject(
		ctx,
		req.BucketName,
		objectPath,
		src,
		req.File.Size,
		storage.PutOptions{},
	); err != nil {
		return err
	}
	chunk := model.FileChunk{
		UploadID:   req.UploadID,
		ChunkIndex: req.ChunkIndex,
		ChunkSize:  req.File.Size,
		ChunkPath:  objectPath,
		Status:     1,
	}
	// 并发上传时 同一个分片被多次提交 导致数据库的混乱 所以需要幂等
	//
	return repo.Db.
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "upload_id"},
				{Name: "chunk_index"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"chunk_size",
				"chunk_path",
				"status",
				"updated_at",
			}),
		}).
		Create(&chunk).Error
}

// FindAllChunkFile loads all chunks for completion.
func FindAllChunkFile(userID uint64, chunks *[]model.FileChunk, req dto.MultipartCompleteRequest) error {
	var session model.UploadSession
	if err := repo.Db.Where("file_hash = ? AND user_id = ?", req.FileHash, userID).Order("id desc").First(&session).Error; err != nil {
		return err
	}
	return repo.Db.
		Where("upload_id = ? AND status = 1", session.UploadID).
		Order("chunk_index asc").
		Find(chunks).Error
}

// CompleteFile composes chunks and creates file records.
func CompleteFile(
	ctx context.Context,
	req dto.MultipartCompleteRequest,
	userName string,
) error {
	userId, err := FindIdByUsername(userName)
	if err != nil {
		return err
	}
	chunks := make([]model.FileChunk, 0)
	if err := FindAllChunkFile(userId, &chunks, req); err != nil {
		return err
	}
	if len(chunks) != req.TotalChunks {
		return errors.New("chunks not complete")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if storage.Default == nil {
		return fmt.Errorf("storage not initialized")
	}
	cleanupUploadData := func() { // 删除所有 chunk session 等
		for _, c := range chunks {
			_ = storage.Default.RemoveObject(ctx, config.AppConfig.BucketName, c.ChunkPath)
		}
		var session model.UploadSession
		if err := repo.Db.Where("file_hash = ? AND user_id = ?", req.FileHash, userId).Order("id desc").First(&session).Error; err == nil {
			repo.Db.Where("upload_id = ?", session.UploadID).Delete(&model.FileChunk{})
			repo.Db.Delete(&session)
		}
	}
	writeObject := func(objectName string) error {
		if req.TotalChunks == 0 {
			if req.FileSize != 0 {
				return errors.New("invalid total_chunks")
			}
			return storage.Default.PutObject(
				ctx,
				config.AppConfig.BucketName,
				objectName,
				bytes.NewReader(nil),
				0,
				storage.PutOptions{},
			)
		}
		srcs := make([]storage.CopySource, 0, len(chunks))
		for _, c := range chunks {
			srcs = append(srcs, storage.CopySource{
				Bucket: config.AppConfig.BucketName,
				Object: c.ChunkPath,
			})
		}
		dst := storage.CopyDest{
			Bucket: config.AppConfig.BucketName,
			Object: objectName,
		}
		return storage.Default.ComposeObject(ctx, dst, srcs...) // 调用 minio 客户端api
	}

	var (
		objectID         uint64
		dstObject        string
		createdNewObject bool
		increasedRef     bool
	)
	existingObj, err := GetFileObjectByHash(req.FileHash)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		available, checkErr := isFileObjectAvailable(ctx, existingObj)
		if checkErr != nil {
			return checkErr
		}
		if !available { // hash 存在 但对象不可用 更新并刷新缓存
			oldBucket := existingObj.BucketName
			oldObject := existingObj.ObjectName
			dstObject = existingObj.ObjectName
			if err := writeObject(dstObject); err != nil {
				return err
			}
			if err := repo.Db.Model(&model.FileObject{}).
				Where("id = ?", existingObj.ID).
				Updates(map[string]interface{}{
					"bucket_name": config.AppConfig.BucketName,
					"object_name": dstObject,
					"size":        req.FileSize,
				}).Error; err != nil {
				return err
			}
			existingObj.BucketName = config.AppConfig.BucketName
			existingObj.ObjectName = dstObject
			existingObj.Size = req.FileSize
			if oldBucket != existingObj.BucketName || oldObject != existingObj.ObjectName {
				_ = utils.InvalidateFileObjectPathCache(ctx, oldBucket, oldObject)
			}
			cacheFileObject(ctx, existingObj)
		}
		// hash 存在＋对象可用
		if err := IncreaseRefCount(existingObj.ID); err != nil {
			return err
		}
		increasedRef = true
		objectID = existingObj.ID
	} else {
		dstObject = BuildObjectName(userName, req.FileHash)
		if err := writeObject(dstObject); err != nil {
			return err
		}
		obj := &model.FileObject{
			UserID:     userId,
			BucketName: config.AppConfig.BucketName,
			Hash:       req.FileHash,
			ObjectName: dstObject,
			Size:       req.FileSize,
			RefCount:   1,
		}
		if err := CreateFilesObject(obj); err != nil { // 回滚
			_ = storage.Default.RemoveObject(ctx, config.AppConfig.BucketName, dstObject)
			return err
		}
		objectID = obj.ID
		createdNewObject = true
	}

	var parentID *uint64
	if req.ParentId != 0 {
		parentID = &req.ParentId
	}
	userFile := &model.UserFile{
		UserID:   userId,
		Name:     req.FileName,
		ParentID: parentID,
		IsDir:    false,
		ObjectID: &objectID,
		Size:     req.FileSize,
	}
	if err := CreateUserFileEntry(userFile); err != nil { // 回滚
		if createdNewObject {
			_ = storage.Default.RemoveObject(ctx, config.AppConfig.BucketName, dstObject)
			_ = repo.Db.Delete(&model.FileObject{}, objectID).Error
		}
		if increasedRef {
			_, _ = DecreaseRefCount(objectID)
		}
		return err
	}

	cleanupUploadData()
	return nil
}

// FindObjectIdByName finds object ID by name.
func FindObjectIdByName(name string) (uint64, error) {
	var fileObject model.FileObject
	if err := repo.Db.Where("object_name = ?", name).First(&fileObject).Error; err != nil {
		return 0, err
	}
	return fileObject.ID, nil
}

// RemoveObject reduces ref count and deletes object if needed.
func RemoveObject(objectId uint64) error {
	var fileObject model.FileObject
	if err := repo.Db.Where("id = ?", objectId).First(&fileObject).Error; err != nil {
		return err
	}
	remain, err := DecreaseRefCount(objectId)
	if err != nil {
		return err
	}
	if remain > 0 {
		return nil
	}
	// 如果是最后一个 则清理数据
	if err := DeleteMinioFile(&fileObject); err != nil {
		return err
	}
	if err := repo.Db.Delete(&model.FileObject{}, objectId).Error; err != nil {
		return err
	}
	_ = utils.InvalidateFileObjectCache(context.Background(), objectId)
	_ = utils.InvalidateFileObjectHashCache(context.Background(), fileObject.Hash)
	_ = utils.InvalidateFileObjectPathCache(context.Background(), fileObject.BucketName, fileObject.ObjectName)
	var session model.UploadSession
	if err := repo.Db.Where("file_hash = ? AND user_id = ?", fileObject.Hash, fileObject.UserID).Order("id desc").First(&session).Error; err != nil {
		return nil
	}
	return repo.Db.Where("upload_id = ?", session.UploadID).Delete(&model.FileChunk{}).Error
}
