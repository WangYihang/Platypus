-- install downloads can now mint enrollment tokens with configurable
-- max_uses instead of the historical hard-coded single use.
ALTER TABLE install_download_tokens ADD COLUMN pat_max_uses INTEGER NOT NULL DEFAULT 1
    CHECK (pat_max_uses >= 1);
