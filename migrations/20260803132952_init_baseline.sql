-- Create "answers" table
CREATE TABLE "answers" (
  "id" uuid NOT NULL,
  "test_result_id" uuid NOT NULL,
  "question_id" uuid NOT NULL,
  "value" text NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_ans_test_quest" to table: "answers"
CREATE UNIQUE INDEX "idx_ans_test_quest" ON "answers" ("test_result_id", "question_id");
-- Create "data_deletion_requests" table
CREATE TABLE "data_deletion_requests" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "notification_email" text NOT NULL,
  "status" text NOT NULL,
  "requested_at" timestamptz NOT NULL,
  "completed_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_data_deletion_requests_user_id" to table: "data_deletion_requests"
CREATE INDEX "idx_data_deletion_requests_user_id" ON "data_deletion_requests" ("user_id");
-- Create index "uniq_active_deletion_per_user" to table: "data_deletion_requests"
CREATE UNIQUE INDEX "uniq_active_deletion_per_user" ON "data_deletion_requests" ("user_id") WHERE ((status = 'pending_grace'::text) OR (status = 'processing'::text));
-- Create "guest_sessions" table
CREATE TABLE "guest_sessions" (
  "session_id" uuid NOT NULL,
  "ip_hash" text NOT NULL,
  "display_name" text NOT NULL,
  "age" bigint NOT NULL,
  "status" text NOT NULL,
  "locale" text NOT NULL DEFAULT 'en',
  "claimed_by_user_id" uuid NULL,
  "created_at" timestamptz NULL,
  "expires_at" timestamptz NULL,
  PRIMARY KEY ("session_id")
);
-- Create index "idx_guest_sessions_claimed_by_user_id" to table: "guest_sessions"
CREATE INDEX "idx_guest_sessions_claimed_by_user_id" ON "guest_sessions" ("claimed_by_user_id");
-- Create index "idx_guest_sessions_expires_at" to table: "guest_sessions"
CREATE INDEX "idx_guest_sessions_expires_at" ON "guest_sessions" ("expires_at");
-- Create "insight_templates" table
CREATE TABLE "insight_templates" (
  "id" uuid NOT NULL,
  "insight_key" character varying(100) NOT NULL,
  "locale" character varying(10) NOT NULL,
  "trait" character varying(10) NOT NULL,
  "condition_type" character varying(20) NOT NULL,
  "min_delta" numeric NULL,
  "threshold_value" numeric NULL,
  "template_text" text NOT NULL,
  "is_active" boolean NOT NULL DEFAULT true,
  PRIMARY KEY ("id")
);
-- Create index "idx_it_key_locale" to table: "insight_templates"
CREATE UNIQUE INDEX "idx_it_key_locale" ON "insight_templates" ("insight_key", "locale");
-- Create "prompt_audit_logs" table
CREATE TABLE "prompt_audit_logs" (
  "id" uuid NOT NULL,
  "test_result_id" uuid NOT NULL,
  "raw_prompt" text NOT NULL,
  "raw_response" text NOT NULL,
  "flagged_anomaly" boolean NOT NULL DEFAULT false,
  "created_at" timestamptz NULL,
  "expires_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_prompt_audit_logs_expires_at" to table: "prompt_audit_logs"
CREATE INDEX "idx_prompt_audit_logs_expires_at" ON "prompt_audit_logs" ("expires_at");
-- Create index "idx_prompt_audit_logs_test_result_id" to table: "prompt_audit_logs"
CREATE INDEX "idx_prompt_audit_logs_test_result_id" ON "prompt_audit_logs" ("test_result_id");
-- Create "question_translations" table
CREATE TABLE "question_translations" (
  "id" uuid NOT NULL,
  "question_id" uuid NOT NULL,
  "locale" character varying(10) NOT NULL,
  "question_text" text NOT NULL,
  "options" jsonb NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_qt_quest_locale" to table: "question_translations"
CREATE UNIQUE INDEX "idx_qt_quest_locale" ON "question_translations" ("question_id", "locale");
-- Create "questions" table
CREATE TABLE "questions" (
  "id" uuid NOT NULL,
  "section" character varying(10) NOT NULL,
  "type" character varying(20) NOT NULL,
  "is_reverse_scored" boolean NOT NULL DEFAULT false,
  "is_attention_check" boolean NOT NULL DEFAULT false,
  "display_order" bigint NOT NULL,
  "trait" character varying(10) NOT NULL DEFAULT '',
  "option_trait_map" jsonb NULL,
  PRIMARY KEY ("id")
);
-- Create "referral_codes" table
CREATE TABLE "referral_codes" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "code" text NOT NULL,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_referral_codes_code" to table: "referral_codes"
CREATE UNIQUE INDEX "idx_referral_codes_code" ON "referral_codes" ("code");
-- Create index "idx_referral_codes_user_id" to table: "referral_codes"
CREATE UNIQUE INDEX "idx_referral_codes_user_id" ON "referral_codes" ("user_id");
-- Create "referral_events" table
CREATE TABLE "referral_events" (
  "id" uuid NOT NULL,
  "referral_code_id" uuid NOT NULL,
  "referred_user_id" uuid NOT NULL,
  "event_type" text NOT NULL,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_referral_events_referral_code_id" to table: "referral_events"
CREATE INDEX "idx_referral_events_referral_code_id" ON "referral_events" ("referral_code_id");
-- Create index "idx_referral_events_referred_user_id" to table: "referral_events"
CREATE INDEX "idx_referral_events_referred_user_id" ON "referral_events" ("referred_user_id");
-- Create "test_results" table
CREATE TABLE "test_results" (
  "id" uuid NOT NULL,
  "user_id" uuid NULL,
  "guest_session_id" uuid NULL,
  "share_token" character varying(50) NOT NULL,
  "locale" text NOT NULL,
  "mascot_style" text NOT NULL DEFAULT 'style_a',
  "mbti_type" character varying(4) NULL,
  "grit_score" bigint NULL,
  "trait_scores" jsonb NULL,
  "ai_summary_text" text NULL,
  "status" text NOT NULL,
  "wellbeing_flag" boolean NULL DEFAULT false,
  "pdf_url" text NULL,
  "pdf_status" text NOT NULL DEFAULT 'pending',
  "prompt_tokens" bigint NULL,
  "completion_tokens" bigint NULL,
  "total_tokens" bigint NULL,
  "created_at" timestamptz NULL,
  "expires_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_test_results_created_at" to table: "test_results"
CREATE INDEX "idx_test_results_created_at" ON "test_results" ("created_at");
-- Create index "idx_test_results_expires_at" to table: "test_results"
CREATE INDEX "idx_test_results_expires_at" ON "test_results" ("expires_at");
-- Create index "idx_test_results_guest_session_id" to table: "test_results"
CREATE INDEX "idx_test_results_guest_session_id" ON "test_results" ("guest_session_id");
-- Create index "idx_test_results_share_token" to table: "test_results"
CREATE UNIQUE INDEX "idx_test_results_share_token" ON "test_results" ("share_token");
-- Create index "idx_test_results_user_id" to table: "test_results"
CREATE INDEX "idx_test_results_user_id" ON "test_results" ("user_id");
-- Create "users" table
CREATE TABLE "users" (
  "id" uuid NOT NULL,
  "email" text NOT NULL,
  "password_hash" text NOT NULL,
  "display_name" text NOT NULL DEFAULT '',
  "age" bigint NOT NULL DEFAULT 0,
  "status" text NOT NULL DEFAULT '',
  "referred_by_code" text NULL,
  "preferred_locale" text NOT NULL DEFAULT 'en',
  "email_verified_at" timestamptz NULL,
  "created_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "anonymized_at" timestamptz NULL,
  "token_version" bigint NOT NULL DEFAULT 0,
  "failed_login_count" bigint NOT NULL DEFAULT 0,
  "locked_until" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_users_deleted_at" to table: "users"
CREATE INDEX "idx_users_deleted_at" ON "users" ("deleted_at");
-- Create index "idx_users_email" to table: "users"
CREATE UNIQUE INDEX "idx_users_email" ON "users" ("email");
-- Create index "idx_users_referred_by_code" to table: "users"
CREATE INDEX "idx_users_referred_by_code" ON "users" ("referred_by_code");
-- Create "verification_tokens" table
CREATE TABLE "verification_tokens" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "token" text NOT NULL,
  "type" text NOT NULL,
  "attempt_count" bigint NOT NULL DEFAULT 0,
  "expires_at" timestamptz NOT NULL,
  "used_at" timestamptz NULL,
  "created_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_vt_user_type" to table: "verification_tokens"
CREATE INDEX "idx_vt_user_type" ON "verification_tokens" ("user_id", "type");
