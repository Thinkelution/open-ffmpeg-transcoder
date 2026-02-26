CREATE TABLE presets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',

    -- Full settings JSONB (video, audio, format, hardware_accel, extra_flags)
    settings JSONB NOT NULL,

    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_presets_name ON presets(name);

-- Insert default system presets
INSERT INTO presets (name, description, settings, is_system) VALUES
(
    'h264_1080p',
    'H.264 1080p @ 5Mbps, AAC 192k',
    '{
        "video": {"codec": "libx264", "bitrate": "5M", "width": 1920, "height": 1080, "framerate": 30, "profile": "high", "pixel_format": "yuv420p"},
        "audio": {"codec": "aac", "bitrate": "192k", "channels": 2, "sample_rate": 48000},
        "format": "mp4",
        "hardware_accel": "auto",
        "extra_flags": ["-movflags", "+faststart"]
    }',
    TRUE
),
(
    'h264_720p',
    'H.264 720p @ 2.5Mbps, AAC 128k',
    '{
        "video": {"codec": "libx264", "bitrate": "2500k", "width": 1280, "height": 720, "framerate": 30, "profile": "main", "pixel_format": "yuv420p"},
        "audio": {"codec": "aac", "bitrate": "128k", "channels": 2, "sample_rate": 48000},
        "format": "mp4",
        "hardware_accel": "auto",
        "extra_flags": ["-movflags", "+faststart"]
    }',
    TRUE
),
(
    'h264_480p',
    'H.264 480p @ 1Mbps, AAC 128k',
    '{
        "video": {"codec": "libx264", "bitrate": "1M", "width": 854, "height": 480, "framerate": 30, "profile": "main", "pixel_format": "yuv420p"},
        "audio": {"codec": "aac", "bitrate": "128k", "channels": 2, "sample_rate": 44100},
        "format": "mp4",
        "hardware_accel": "auto",
        "extra_flags": ["-movflags", "+faststart"]
    }',
    TRUE
),
(
    'h265_4k',
    'H.265/HEVC 4K @ 15Mbps, AAC 256k',
    '{
        "video": {"codec": "libx265", "bitrate": "15M", "width": 3840, "height": 2160, "framerate": 30, "profile": "main", "pixel_format": "yuv420p"},
        "audio": {"codec": "aac", "bitrate": "256k", "channels": 2, "sample_rate": 48000},
        "format": "mp4",
        "hardware_accel": "auto",
        "extra_flags": ["-movflags", "+faststart", "-tag:v", "hvc1"]
    }',
    TRUE
),
(
    'nvenc_h264_1080p',
    'NVIDIA NVENC H.264 1080p @ 5Mbps (GPU accelerated)',
    '{
        "video": {"codec": "h264_nvenc", "bitrate": "5M", "width": 1920, "height": 1080, "framerate": 30, "profile": "high", "pixel_format": "yuv420p"},
        "audio": {"codec": "aac", "bitrate": "192k", "channels": 2, "sample_rate": 48000},
        "format": "mp4",
        "hardware_accel": "nvidia",
        "extra_flags": ["-movflags", "+faststart"]
    }',
    TRUE
),
(
    'nvenc_h265_4k',
    'NVIDIA NVENC H.265 4K @ 15Mbps (GPU accelerated)',
    '{
        "video": {"codec": "hevc_nvenc", "bitrate": "15M", "width": 3840, "height": 2160, "framerate": 30, "profile": "main", "pixel_format": "yuv420p"},
        "audio": {"codec": "aac", "bitrate": "256k", "channels": 2, "sample_rate": 48000},
        "format": "mp4",
        "hardware_accel": "nvidia",
        "extra_flags": ["-movflags", "+faststart", "-tag:v", "hvc1"]
    }',
    TRUE
),
(
    'audio_mp3',
    'MP3 audio extraction @ 320kbps',
    '{
        "video": {},
        "audio": {"codec": "libmp3lame", "bitrate": "320k", "channels": 2, "sample_rate": 44100},
        "format": "mp3",
        "hardware_accel": "none",
        "extra_flags": ["-vn"]
    }',
    TRUE
),
(
    'audio_aac',
    'AAC audio extraction @ 256kbps',
    '{
        "video": {},
        "audio": {"codec": "aac", "bitrate": "256k", "channels": 2, "sample_rate": 48000},
        "format": "m4a",
        "hardware_accel": "none",
        "extra_flags": ["-vn"]
    }',
    TRUE
);
