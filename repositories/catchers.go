package repositories

import (
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Catcher struct {
	ID                 int
	LicensePlateNumber string
	UserID             string
	UserName           string
	SelfIntro          string
	HauntedPlaces      string
	CoverURL           string
	GroupID            string
	GroupName          string
}

type WildCatcher struct {
	ID                 int
	LicensePlateNumber string
	Count              int
}

type CatchersRepository interface {
	Create(catcher Catcher) (int, error)
	SearchByLicensePlateNumber(groupID, licensePlateNumber string) ([]Catcher, error)
	IncreaseWildCatcher(licensePlateNumber string) (int, error)
}

type catcherRepository struct {
	db *gorm.DB
}

func NewCatcherRepository() CatchersRepository {
	db, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	return &catcherRepository{db: db}
}

func (r *catcherRepository) Create(catcher Catcher) (int, error) {
	return catcher.ID, r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"license_plate_number", "user_name", "haunted_places", "self_intro", "cover_url", "group_name"}),
	}).Create(&catcher).Error
}

func (r *catcherRepository) SearchByLicensePlateNumber(groupID, licensePlateNumber string) ([]Catcher, error) {
	var result []Catcher
	return result, r.db.
		Where("license_plate_number like ?", "%"+licensePlateNumber+"%").
		Find(&result).Error
}

func (r *catcherRepository) IncreaseWildCatcher(licensePlateNumber string) (int, error) {
	var wildCatcher WildCatcher

	if err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "license_plate_number"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"count": gorm.Expr("wild_catchers.count + ?", 1)}),
	}).Create(&WildCatcher{
		LicensePlateNumber: licensePlateNumber,
		Count:              1,
	}).Error; err != nil {
		return 0, err
	}

	return wildCatcher.Count, r.db.Where("license_plate_number = ?", licensePlateNumber).First(&wildCatcher).Error
}
