package main

import (
	"log"

	"github.com/aprxty3/your_persona_controller.git/internal/config"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	log.Println("Connecting to database for seeding...")
	db, err := postgres.NewPostgresDB(config.PostgresDSN())
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
		{
			ID:            "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d15",
			InsightKey:    "grit_decrease",
			Locale:        "en",
			Trait:         "grit",
			ConditionType: "decrease",
			MinDelta:      floatPtr(5.0),
			TemplateText:  "Your GRIT score dipped by {delta} points since last time — a normal part of the journey. Reflecting on what changed can help you regain momentum.",
			IsActive:      true,
		},
		{
			ID:            "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d16",
			InsightKey:    "grit_decrease",
			Locale:        "id",
			Trait:         "grit",
			ConditionType: "decrease",
			MinDelta:      floatPtr(5.0),
			TemplateText:  "Skor GRIT Anda turun {delta} poin sejak terakhir kali — ini bagian normal dari perjalanan. Merenungkan apa yang berubah bisa membantu Anda membangun kembali momentum.",
			IsActive:      true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d17",
			InsightKey:     "ei_extrovert_strength",
			Locale:         "en",
			Trait:          "ei",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "You draw energy from social interaction and tend to think out loud — a natural fit for collaborative, people-facing work.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d18",
			InsightKey:     "ei_extrovert_strength",
			Locale:         "id",
			Trait:          "ei",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Anda mendapat energi dari interaksi sosial dan cenderung berpikir sambil bicara — cocok untuk pekerjaan kolaboratif yang banyak berhubungan dengan orang.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d19",
			InsightKey:     "ei_extrovert_blindspot",
			Locale:         "en",
			Trait:          "ei",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Because you lean strongly toward Extroversion, it's worth intentionally carving out quiet time to process ideas before jumping into discussion.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d1a",
			InsightKey:     "ei_extrovert_blindspot",
			Locale:         "id",
			Trait:          "ei",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Karena kecenderungan Ekstroversi Anda cukup kuat, ada baiknya sengaja menyediakan waktu tenang untuk memproses ide sebelum langsung terjun ke diskusi.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d1b",
			InsightKey:     "sn_sensing_strength",
			Locale:         "en",
			Trait:          "sn",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "You have a strong eye for concrete details and practical realities — reliable when a task needs careful, grounded execution.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d1c",
			InsightKey:     "sn_sensing_strength",
			Locale:         "id",
			Trait:          "sn",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Anda punya kepekaan kuat terhadap detail konkret dan realita praktis — bisa diandalkan saat sebuah tugas butuh eksekusi yang teliti dan membumi.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d1d",
			InsightKey:     "sn_sensing_blindspot",
			Locale:         "en",
			Trait:          "sn",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "With a strong focus on Sensing, try to periodically step back and consider the bigger picture or long-term possibilities, not just what's directly in front of you.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d1e",
			InsightKey:     "sn_sensing_blindspot",
			Locale:         "id",
			Trait:          "sn",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Dengan fokus kuat pada Sensing, sesekali coba melangkah mundur dan pikirkan gambaran besar atau kemungkinan jangka panjang, bukan cuma yang ada di depan mata.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d1f",
			InsightKey:     "tf_thinking_strength",
			Locale:         "en",
			Trait:          "tf",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "You approach decisions with logic and objective analysis — a steady hand when a situation calls for clear-headed judgment.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d20",
			InsightKey:     "tf_thinking_strength",
			Locale:         "id",
			Trait:          "tf",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Anda mengambil keputusan dengan logika dan analisis objektif — sosok yang tenang saat situasi butuh penilaian yang jernih.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d21",
			InsightKey:     "tf_thinking_blindspot",
			Locale:         "en",
			Trait:          "tf",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Since you lean strongly toward Thinking, remember to also check in on how a decision lands emotionally for the people involved.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d22",
			InsightKey:     "tf_thinking_blindspot",
			Locale:         "id",
			Trait:          "tf",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Karena kecenderungan Thinking Anda cukup kuat, jangan lupa juga mempertimbangkan bagaimana sebuah keputusan terasa secara emosional bagi orang-orang yang terlibat.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d23",
			InsightKey:     "jp_judging_strength",
			Locale:         "en",
			Trait:          "jp",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "You like structure, plans, and closure — things tend to get finished when you're involved.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d24",
			InsightKey:     "jp_judging_strength",
			Locale:         "id",
			Trait:          "jp",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Anda menyukai struktur, rencana, dan penyelesaian — sesuatu cenderung benar-benar rampung kalau Anda terlibat.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d25",
			InsightKey:     "jp_judging_blindspot",
			Locale:         "en",
			Trait:          "jp",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "With a strong preference for Judging, stay open to adjusting the plan when new information shows up — not every situation needs to be settled immediately.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d26",
			InsightKey:     "jp_judging_blindspot",
			Locale:         "id",
			Trait:          "jp",
			ConditionType:  "threshold",
			ThresholdValue: floatPtr(60.0),
			TemplateText:   "Dengan preferensi Judging yang kuat, tetap terbuka untuk menyesuaikan rencana kalau ada informasi baru — tidak semua situasi harus langsung diputuskan saat itu juga.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d27",
			InsightKey:     "ei_introvert_strength",
			Locale:         "en",
			Trait:          "ei",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "You recharge through solitude and think before you speak — a natural fit for deep, focused work and thoughtful one-on-one connections.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d28",
			InsightKey:     "ei_introvert_strength",
			Locale:         "id",
			Trait:          "ei",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Anda mengisi ulang energi lewat kesendirian dan berpikir dulu sebelum bicara — cocok untuk pekerjaan mendalam yang butuh fokus dan hubungan satu-lawan-satu yang bermakna.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d29",
			InsightKey:     "ei_introvert_blindspot",
			Locale:         "en",
			Trait:          "ei",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "With a strong Introversion preference, your ideas can stay invisible in group settings — practice sharing early drafts out loud so your thinking gets the credit it deserves.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d2a",
			InsightKey:     "ei_introvert_blindspot",
			Locale:         "id",
			Trait:          "ei",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Dengan preferensi Introversion yang kuat, ide Anda bisa tak terlihat di forum ramai — coba biasakan menyuarakan draf pemikiran lebih awal supaya kontribusi Anda mendapat tempat yang layak.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d2b",
			InsightKey:     "sn_intuition_strength",
			Locale:         "en",
			Trait:          "sn",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "You naturally see patterns, possibilities, and the bigger picture — a strength for strategy, innovation, and imagining what could be.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d2c",
			InsightKey:     "sn_intuition_strength",
			Locale:         "id",
			Trait:          "sn",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Anda peka melihat pola, kemungkinan, dan gambaran besar — kekuatan untuk strategi, inovasi, dan membayangkan apa yang mungkin terjadi.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d2d",
			InsightKey:     "sn_intuition_blindspot",
			Locale:         "en",
			Trait:          "sn",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "With a strong Intuition preference, everyday details can slip past you — pairing big ideas with a simple checklist keeps execution as strong as the vision.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d2e",
			InsightKey:     "sn_intuition_blindspot",
			Locale:         "id",
			Trait:          "sn",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Dengan preferensi Intuition yang kuat, detail keseharian bisa terlewat — memasangkan ide besar dengan checklist sederhana membuat eksekusi sekuat visinya.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d2f",
			InsightKey:     "tf_feeling_strength",
			Locale:         "en",
			Trait:          "tf",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "You weigh decisions by their impact on people and values — a strength for building trust, empathy, and teams where everyone feels heard.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d30",
			InsightKey:     "tf_feeling_strength",
			Locale:         "id",
			Trait:          "tf",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Anda menimbang keputusan dari dampaknya ke orang dan nilai-nilai — kekuatan untuk membangun kepercayaan, empati, dan tim yang setiap anggotanya merasa didengar.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d31",
			InsightKey:     "tf_feeling_blindspot",
			Locale:         "en",
			Trait:          "tf",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "With a strong Feeling preference, hard calls that disappoint someone can weigh heavily — remember that clear, kind honesty often serves people better than kept peace.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d32",
			InsightKey:     "tf_feeling_blindspot",
			Locale:         "id",
			Trait:          "tf",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Dengan preferensi Feeling yang kuat, keputusan sulit yang mengecewakan seseorang bisa terasa berat — ingat, kejujuran yang jelas dan baik hati sering lebih menolong daripada sekadar menjaga suasana.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d33",
			InsightKey:     "jp_perceiving_strength",
			Locale:         "en",
			Trait:          "jp",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "You stay open and adaptable when plans change — a strength for improvising, exploring options, and thriving in situations others find chaotic.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d34",
			InsightKey:     "jp_perceiving_strength",
			Locale:         "id",
			Trait:          "jp",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Anda tetap terbuka dan adaptif saat rencana berubah — kekuatan untuk berimprovisasi, menjajaki opsi, dan tetap nyaman di situasi yang orang lain anggap kacau.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d35",
			InsightKey:     "jp_perceiving_blindspot",
			Locale:         "en",
			Trait:          "jp",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "With a strong Perceiving preference, deadlines can turn into last-minute sprints — setting your own earlier checkpoint keeps the flexibility without the scramble.",
			IsActive:       true,
		},
		{
			ID:             "d0eebc99-9c0b-4ef8-bb6d-6bb9bd380d36",
			InsightKey:     "jp_perceiving_blindspot",
			Locale:         "id",
			Trait:          "jp",
			ConditionType:  "threshold_below",
			ThresholdValue: floatPtr(40.0),
			TemplateText:   "Dengan preferensi Perceiving yang kuat, tenggat bisa berubah jadi sprint menit terakhir — membuat checkpoint pribadi yang lebih awal menjaga fleksibilitas tanpa kerepotan di ujung.",
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
