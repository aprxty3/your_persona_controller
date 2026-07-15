package main

import (
	"fmt"
	"log"
	"os"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbHost := os.Getenv("DB_HOST")
		if dbHost == "" {
			dbHost = "localhost"
		}
		dbPort := os.Getenv("DB_PORT")
		if dbPort == "" {
			dbPort = "5432"
		}
		dbUser := os.Getenv("DB_USER")
		if dbUser == "" {
			dbUser = "postgres"
		}
		dbPassword := os.Getenv("DB_PASSWORD")
		if dbPassword == "" {
			dbPassword = "changeme"
		}
		dbName := os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "psyche_assessment"
		}
		dbSSLMode := os.Getenv("DB_SSLMODE")
		if dbSSLMode == "" {
			dbSSLMode = "disable"
		}
		dbDSN = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
			dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)
	}

	log.Println("Connecting to database for seeding...")
	db, err := postgres.NewPostgresDB(dbDSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Seeding questions...")
	if err := seedQuestions(db); err != nil {
		log.Fatalf("Failed to seed questions: %v", err)
	}

	log.Println("Seeding insight templates...")
	if err := seedInsightTemplates(db); err != nil {
		log.Fatalf("Failed to seed insight templates: %v", err)
	}

	log.Println("Database seeding completed successfully!")
}

func seedQuestions(db *gorm.DB) error {
	// SJT option → dimension point maps. Positive points lean toward
	// the dimension's FIRST pole (E/S/T/J), negative toward the second (I/N/F/P).
	sjtTraitMap1 := `{"A":{"EI":2},"B":{"EI":-2},"C":{"TF":-1},"D":{"EI":-1},"E":{}}`
	sjtTraitMap2 := `{"A":{"JP":1},"B":{"TF":1},"C":{"EI":1},"D":{"JP":-1},"E":{}}`
	sjtTraitMap3 := `{"A":{"TF":1,"EI":1},"B":{},"C":{"TF":-1},"D":{"JP":1},"E":{}}`

	questions := []postgres.QuestionModel{
		// Section A - SJT (mc) — scored via option_trait_map, not trait
		{ID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Section: "A", Type: "mc", DisplayOrder: 1, OptionTraitMap: &sjtTraitMap1},
		{ID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", Section: "A", Type: "mc", DisplayOrder: 2, OptionTraitMap: &sjtTraitMap2},
		{ID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13", Section: "A", Type: "mc", DisplayOrder: 3, OptionTraitMap: &sjtTraitMap3},

		// Section B - Likert — each item measures exactly one dimension (trait)
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b11", Section: "B", Type: "likert", DisplayOrder: 4, Trait: "EI"},
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b12", Section: "B", Type: "likert", DisplayOrder: 5, Trait: "SN"},
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b13", Section: "B", Type: "likert", DisplayOrder: 6, Trait: "TF"},
		// Reverse-scored
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b14", Section: "B", Type: "likert", IsReverseScored: true, DisplayOrder: 7, Trait: "GRIT"},
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b15", Section: "B", Type: "likert", DisplayOrder: 8, Trait: "JP"},
		// Reverse-scored: statement measures the N pole, so agreement must move away from S
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b16", Section: "B", Type: "likert", IsReverseScored: true, DisplayOrder: 9, Trait: "SN"},
		// Attention check  — excluded from scoring entirely, no trait
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b17", Section: "B", Type: "likert", IsAttentionCheck: true, DisplayOrder: 10},
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b18", Section: "B", Type: "likert", DisplayOrder: 11, Trait: "GRIT"},
		// Reverse-scored: classic Duckworth Grit Scale reverse-keyed item — agreement = lower grit
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b19", Section: "B", Type: "likert", IsReverseScored: true, DisplayOrder: 12, Trait: "GRIT"},
		// Reverse-scored
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b20", Section: "B", Type: "likert", IsReverseScored: true, DisplayOrder: 13, Trait: "GRIT"},

		// Section C - Essay Prompt — analyzed by Gemini, never scored numerically
		{ID: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c11", Section: "C", Type: "essay_prompt", DisplayOrder: 14},
		{ID: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c12", Section: "C", Type: "essay_prompt", DisplayOrder: 15},
	}

	// Idempotent upsert
	for _, q := range questions {
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"section", "type", "is_reverse_scored", "is_attention_check", "display_order", "trait", "option_trait_map"}),
		}).Create(&q).Error
		if err != nil {
			return err
		}
	}

	// Translations helper
	sjtOptionsEN1 := `["Take control and lead the group", "Wait for someone else to lead", "Offer to compromise", "Do your own work independently", "Ask the supervisor for guidance"]`
	sjtOptionsID1 := `["Mengambil kendali dan memimpin kelompok", "Menunggu orang lain memimpin", "Menawarkan kompromi", "Mengerjakan tugas sendiri secara mandiri", "Meminta petunjuk dari atasan"]`

	sjtOptionsEN2 := `["Work late to finish it", "Ask for an extension immediately", "Delegate work to others", "Do a rushed job to meet the deadline", "Panic and do nothing"]`
	sjtOptionsID2 := `["Bekerja lembur untuk menyelesaikannya", "Segera meminta perpanjangan waktu", "Mendelegasikan pekerjaan ke orang lain", "Mengerjakan terburu-buru asal selesai", "Panik dan tidak melakukan apa-apa"]`

	sjtOptionsEN3 := `["Address the issue directly with them", "Complain to other colleagues", "Ignore it and hope it improves", "Report them to HR immediately", "Request to transfer to another team"]`
	sjtOptionsID3 := `["Membahas masalahnya secara langsung dengan mereka", "Mengeluh kepada rekan kerja lain", "Mengabaikannya dan berharap membaik", "Segera melaporkan ke HRD", "Meminta pindah ke tim lain"]`

	likertOptionsEN := `["Strongly Disagree", "Disagree", "Neutral", "Agree", "Strongly Agree"]`
	likertOptionsID := `["Sangat Tidak Setuju", "Tidak Setuju", "Netral", "Setuju", "Sangat Setuju"]`

	translations := []postgres.QuestionTranslationModel{
		// SJT 1
		{
			ID:           "a1eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			QuestionID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			Locale:       "en",
			QuestionText: "Your team is facing a sudden crisis with a tight deadline. How do you respond?",
			Options:      &sjtOptionsEN1,
		},
		{
			ID:           "a1eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			QuestionID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			Locale:       "id",
			QuestionText: "Tim Anda menghadapi krisis mendadak dengan tenggat waktu ketat. Bagaimana Anda merespons?",
			Options:      &sjtOptionsID1,
		},
		// SJT 2
		{
			ID:           "a2eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			QuestionID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			Locale:       "en",
			QuestionText: "You realize you won't be able to complete a critical task on time. What do you do?",
			Options:      &sjtOptionsEN2,
		},
		{
			ID:           "a2eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			QuestionID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			Locale:       "id",
			QuestionText: "Anda menyadari tidak akan bisa menyelesaikan tugas penting tepat waktu. Apa yang Anda lakukan?",
			Options:      &sjtOptionsID2,
		},
		// SJT 3
		{
			ID:           "a3eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			QuestionID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
			Locale:       "en",
			QuestionText: "A coworker is constantly slacking off, affecting your team's throughput. How do you handle it?",
			Options:      &sjtOptionsEN3,
		},
		{
			ID:           "a3eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
			QuestionID:   "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13",
			Locale:       "id",
			QuestionText: "Rekan kerja terus-menerus malas-malasan, mempengaruhi produktivitas tim Anda. Bagaimana Anda menanganinya?",
			Options:      &sjtOptionsID3,
		},

		// Likert 1
		{
			ID:           "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			Locale:       "en",
			QuestionText: "I enjoy being the center of attention in social gatherings.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			Locale:       "id",
			QuestionText: "Saya menikmati menjadi pusat perhatian dalam acara sosial.",
			Options:      &likertOptionsID,
		},
		// Likert 2
		{
			ID:           "b2eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			Locale:       "en",
			QuestionText: "I focus on details and facts rather than abstract concepts.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b2eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			Locale:       "id",
			QuestionText: "Saya fokus pada detail dan fakta daripada konsep abstrak.",
			Options:      &likertOptionsID,
		},
		// Likert 3
		{
			ID:           "b3eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b13",
			Locale:       "en",
			QuestionText: "I prioritize logic over emotions when making difficult decisions.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b3eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b13",
			Locale:       "id",
			QuestionText: "Saya memprioritaskan logika daripada emosi saat membuat keputusan sulit.",
			Options:      &likertOptionsID,
		},
		// Likert 4 (Reverse-scored)
		{
			ID:           "b4eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b14",
			Locale:       "en",
			QuestionText: "I often give up on tasks when they get highly frustrating.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b4eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b14",
			Locale:       "id",
			QuestionText: "Saya sering menyerah pada tugas ketika keadaan menjadi sangat frustrasi.",
			Options:      &likertOptionsID,
		},
		// Likert 5
		{
			ID:           "b5eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b15",
			Locale:       "en",
			QuestionText: "I prefer having a detailed plan before starting a project.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b5eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b15",
			Locale:       "id",
			QuestionText: "Saya lebih suka memiliki rencana terperinci sebelum memulai suatu proyek.",
			Options:      &likertOptionsID,
		},
		// Likert 6
		{
			ID:           "b6eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b16",
			Locale:       "en",
			QuestionText: "I actively seek out new experiences and change.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b6eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b16",
			Locale:       "id",
			QuestionText: "Saya aktif mencari pengalaman baru dan perubahan.",
			Options:      &likertOptionsID,
		},
		// Likert 7 (Attention check)
		{
			ID:           "b7eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b17",
			Locale:       "en",
			QuestionText: "Please select 'Agree' to confirm you are reading this question.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b7eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b17",
			Locale:       "id",
			QuestionText: "Silakan pilih 'Setuju' untuk mengonfirmasi Anda membaca pertanyaan ini.",
			Options:      &likertOptionsID,
		},
		// Likert 8
		{
			ID:           "b8eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b18",
			Locale:       "en",
			QuestionText: "I finish whatever I begin and do not leave things half-done.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b8eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b18",
			Locale:       "id",
			QuestionText: "Saya menyelesaikan apa pun yang saya mulai dan tidak membiarkannya setengah selesai.",
			Options:      &likertOptionsID,
		},
		// Likert 9
		{
			ID:           "b9eebc99-9c0b-4ef8-bb6d-6bb9bd380b11",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b19",
			Locale:       "en",
			QuestionText: "New ideas and projects sometimes distract me from previous ones.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b9eebc99-9c0b-4ef8-bb6d-6bb9bd380b12",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b19",
			Locale:       "id",
			QuestionText: "Ide dan proyek baru terkadang mengalihkan perhatian saya dari proyek sebelumnya.",
			Options:      &likertOptionsID,
		},
		// Likert 10 (Reverse-scored)
		{
			ID:           "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380b21",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b20",
			Locale:       "en",
			QuestionText: "I find it hard to maintain focus on projects that take more than a few months to complete.",
			Options:      &likertOptionsEN,
		},
		{
			ID:           "b1eebc99-9c0b-4ef8-bb6d-6bb9bd380b22",
			QuestionID:   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380b20",
			Locale:       "id",
			QuestionText: "Saya kesulitan mempertahankan fokus pada proyek yang membutuhkan waktu lebih dari beberapa bulan untuk diselesaikan.",
			Options:      &likertOptionsID,
		},

		// Essay 1
		{
			ID:           "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380c11",
			QuestionID:   "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c11",
			Locale:       "en",
			QuestionText: "Tell us about a time you failed to achieve a goal. What did you learn? (Max 4000 characters)",
			Options:      nil,
		},
		{
			ID:           "c1eebc99-9c0b-4ef8-bb6d-6bb9bd380c12",
			QuestionID:   "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c11",
			Locale:       "id",
			QuestionText: "Ceritakan tentang kegagalan Anda dalam mencapai suatu tujuan. Apa yang Anda pelajari? (Maks 4000 karakter)",
			Options:      nil,
		},
		// Essay 2
		{
			ID:           "c2eebc99-9c0b-4ef8-bb6d-6bb9bd380c11",
			QuestionID:   "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c12",
			Locale:       "en",
			QuestionText: "Describe a long-term goal you are currently pursuing. What is your strategy? (Max 4000 characters)",
			Options:      nil,
		},
		{
			ID:           "c2eebc99-9c0b-4ef8-bb6d-6bb9bd380c12",
			QuestionID:   "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380c12",
			Locale:       "id",
			QuestionText: "Gambarkan tujuan jangka panjang yang sedang Anda kejar. Apa strategi Anda? (Maks 4000 karakter)",
			Options:      nil,
		},
	}

	for _, tr := range translations {
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "question_id"}, {Name: "locale"}},
			DoUpdates: clause.AssignmentColumns([]string{"question_text", "options"}),
		}).Create(&tr).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func seedInsightTemplates(db *gorm.DB) error {
	floatPtr := func(f float64) *float64 { return &f }

	templates := []postgres.InsightTemplateModel{
		// GRIT Increase
		{
			ID:            "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d11",
			InsightKey:    "grit_increase",
			Locale:        "en",
			Trait:         "grit",
			ConditionType: "increase",
			MinDelta:      floatPtr(5.0),
			TemplateText:  "Incredible progress! Your GRIT score increased by {delta} points. You are showing stronger resilience and focus.",
			IsActive:      true,
		},
		{
			ID:            "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d12",
			InsightKey:    "grit_increase",
			Locale:        "id",
			Trait:         "grit",
			ConditionType: "increase",
			MinDelta:      floatPtr(5.0),
			TemplateText:  "Kemajuan luar biasa! Skor GRIT Anda meningkat sebesar {delta} poin. Anda menunjukkan ketahanan dan fokus yang lebih kuat.",
			IsActive:      true,
		},

		// GRIT Threshold High
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d13",
			InsightKey:     "grit_high_threshold",
			Locale:         "en",
			Trait:          "grit",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(80.0),
			TemplateText:   "Outstanding! Your GRIT score of {value}% puts you in the top tier of goal-oriented individuals.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d14",
			InsightKey:     "grit_high_threshold",
			Locale:         "id",
			Trait:          "grit",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(80.0),
			TemplateText:   "Luar biasa! Skor GRIT Anda sebesar {value}% menempatkan Anda di jajaran teratas individu yang berorientasi pada tujuan.",
			IsActive:       true,
		},
	}

	for _, t := range templates {
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "insight_key"}, {Name: "locale"}},
			DoUpdates: clause.AssignmentColumns([]string{"trait", "condition_type", "min_delta", "threshold_value", "template_text", "is_active"}),
		}).Create(&t).Error
		if err != nil {
			return err
		}
	}

	return nil
}
