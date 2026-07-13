package database

import "time"

// UserDynamicText is a user-published image/text feed post (动态图文).
type UserDynamicText struct {
	ID         uint64 `gorm:"primaryKey"`
	UserID     uint64 `gorm:"index:idx_dyn_user_created;not null"`
	Title      string `gorm:"size:20;not null;default:''"`
	Content    string `gorm:"size:233;not null;default:''"`
	ImagesJSON    string `gorm:"type:text;not null"`
	LikeCount     uint64 `gorm:"not null;default:0"`
	CommentCount  uint64 `gorm:"not null;default:0"`
	CommentsClosed bool `gorm:"not null;default:0"`
	CreatedAt     time.Time `gorm:"index:idx_dyn_user_created"`
}
