package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alpody/fiber-realworld/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func New() *gorm.DB {
	//dsn := "host=/tmp user=realworld dbname=realworld"
	dsn := "./database/realworld.db"

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Millisecond * 10, // Slow SQL threshold
			LogLevel:                  logger.Info,           // Log level
			IgnoreRecordNotFoundError: false,                 // Ignore ErrRecordNotFound error for logger
			Colorful:                  true,                  // Disable color
		},
	)

	// Globally mode
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})

	/*
	 *db, err := gorm.Open(postgres.New(postgres.Config{
	 *  DSN: dsn,
	 *  //PreferSimpleProtocol: true, // disables implicit prepared statement usage
	 *}), &gorm.Config{})
	 */

	//db, err := gorm.Open("postgresql", "postgresql://realworld@/realworld?host=/tmp")
	//db, err := gorm.Open("sqlite3", "./database/realworld.db")
	if err != nil {
		fmt.Println("storage err: ", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		fmt.Println("storage err: ", err)
	}

	sqlDB.SetMaxIdleConns(3)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db
}

// TestDB returns a fresh, fully isolated in-memory SQLite database.
//
// Hermeticity (was bug #6): the original implementation opened a shared file
// (./../database/realworld_test.db) that survived crashed runs and leaked state
// between runs. Each call here gets a brand-new in-memory database, so every
// setup() / newTestApp() starts from a clean slate with no files on disk.
//
// SetMaxOpenConns(1) is required: a pure ":memory:" database lives for the
// lifetime of a single connection, so the pool must be pinned to one connection
// or AutoMigrate's schema would be invisible to subsequent queries.
func TestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		fmt.Println("storage err: ", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		fmt.Println("storage err: ", err)
	}
	sqlDB.SetMaxOpenConns(1)

	return db
}

// DropTestDB is a no-op now that TestDB is in-memory: there is no file to remove,
// and the database is discarded when its *gorm.DB is garbage-collected. Kept for
// API compatibility with the existing harness. (Previously it os.Remove'd a file
// and tearDown log.Fatal'd on failure — part of bug #6.)
func DropTestDB() error {
	return nil
}

func AutoMigrate(db *gorm.DB) {
	db.AutoMigrate(
		&model.User{},
		&model.Follow{},
		&model.Article{},
		&model.Comment{},
		&model.Tag{},
	)
}
