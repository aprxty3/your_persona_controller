package mailer

// emailTemplate is a locale-specific subject/body pair. Body may contain a
// single %s placeholder (filled with the OTP code) via fmt.Sprintf.
type emailTemplate struct {
	Subject string
	Body    string
}

// otpTemplates holds every OTP email variant, keyed [purpose][locale] — the
// single place OTP copy lives, instead of being scattered across if/else
// branches at the call site. Adding a new purpose or locale means adding a
// map entry here, not touching SendOTP's control flow.
var otpTemplates = map[string]map[string]emailTemplate{
	"otp_verification": {
		"id": {
			Subject: "Your Persona's - Kode Verifikasi",
			Body:    "Halo,\n\nKode verifikasi Anda adalah: %s\n\nKode ini berlaku selama 15 menit. Mohon jangan membagikan kode ini kepada siapa pun.\n\nHormat kami,\nTim Your Persona",
		},
		"en": {
			Subject: "Your Persona's - Verification Code",
			Body:    "Hello,\n\nYour verification code is: %s\n\nIt is valid for 15 minutes. Please do not share this code with anyone.\n\nBest regards,\nYour Persona Team",
		},
	},
	"otp_reset": {
		"id": {
			Subject: "Your Persona's - Permintaan Reset Password",
			Body:    "Halo,\n\nKode verifikasi untuk reset password Anda adalah: %s\n\nKode ini berlaku selama 15 menit. Mohon jangan membagikan kode ini kepada siapa pun.\n\nHormat kami,\nTim Your Persona",
		},
		"en": {
			Subject: "Your Persona's - Password Reset Request",
			Body:    "Hello,\n\nYour password reset verification code is: %s\n\nIt is valid for 15 minutes. Please do not share this code with anyone.\n\nBest regards,\nYour Persona Team",
		},
	},
}

// deletionConfirmedTemplates holds the anonymization-complete notification, keyed by locale.
var deletionConfirmedTemplates = map[string]emailTemplate{
	"id": {
		Subject: "Your Persona's - Penghapusan Data Selesai",
		Body:    "Halo,\n\nSesuai permintaan Anda, data pribadi Anda (email, nama, jawaban esai, dan ringkasan AI) telah dihapus/dianonimkan secara permanen dari sistem kami. Akun Anda tidak lagi dapat digunakan untuk login.\n\nTerima kasih telah menggunakan Your Persona's.\n\nHormat kami,\nTim Your Persona",
	},
	"en": {
		Subject: "Your Persona's - Data Deletion Completed",
		Body:    "Hello,\n\nAs you requested, your personal data (email, name, essay answers, and AI summary) has been permanently deleted/anonymized from our systems. Your account can no longer be used to log in.\n\nThank you for using Your Persona's.\n\nBest regards,\nYour Persona Team",
	},
}
