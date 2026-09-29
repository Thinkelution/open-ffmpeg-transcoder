// Shared utilities
function statusClass(status) {
    const classes = {
        'pending': 'status-pending',
        'downloading': 'status-downloading',
        'transcoding': 'status-transcoding animate-pulse-subtle',
        'uploading': 'status-uploading',
        'completed': 'status-completed',
        'failed': 'status-failed',
        'cancelled': 'status-cancelled',
    };
    return classes[status] || 'status-pending';
}

function scoreColor(score) {
    if (score >= 80) return 'text-emerald-400';
    if (score >= 60) return 'text-yellow-400';
    if (score >= 40) return 'text-orange-400';
    return 'text-red-400';
}

function formatDuration(ms) {
    if (!ms) return '-';
    const s = Math.floor(ms / 1000);
    if (s < 60) return s + 's';
    const m = Math.floor(s / 60);
    return m + 'm ' + (s % 60) + 's';
}

async function apiGet(url) {
    const resp = await fetch(url);
    return resp.json();
}

async function apiPost(url, body) {
    const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : undefined,
    });
    return resp.json();
}

async function apiPut(url, body) {
    const resp = await fetch(url, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : undefined,
    });
    return resp.json();
}

async function apiDelete(url) {
    const resp = await fetch(url, { method: 'DELETE' });
    return resp.json();
}

// Dashboard page
function dashboard() {
    return {
        counts: {},
        jobs: [],
        videos: [],
        uploading: false,
        uploadFiles: [],
        uploadQueue: [],
        uploadResult: '',
        uploadError: '',
        draggingUpload: false,
        selectedVideo: null,
        hlsPlayer: null,
        qualityLevels: [],
        selectedQuality: '-1',
        playerStatus: '',
        shareMessage: '',
        statuses: [
            { key: 'pending', label: 'Pending', color: 'text-gray-300' },
            { key: 'downloading', label: 'Downloading', color: 'text-blue-400' },
            { key: 'transcoding', label: 'Transcoding', color: 'text-purple-400' },
            { key: 'uploading', label: 'Uploading', color: 'text-cyan-400' },
            { key: 'completed', label: 'Completed', color: 'text-emerald-400' },
            { key: 'failed', label: 'Failed', color: 'text-red-400' },
            { key: 'cancelled', label: 'Cancelled', color: 'text-gray-400' },
        ],
        statusClass,

        selectUploadFiles(fileList) {
            const files = Array.from(fileList || []).filter((file) => file && file.name);
            this.uploadFiles = files;
            this.uploadQueue = files.map((file, index) => ({
                id: `${Date.now()}-${index}-${file.name}`,
                file,
                name: file.name,
                size: file.size,
                progress: 0,
                status: 'ready',
                message: '',
                jobId: '',
            }));
            this.uploadResult = '';
            this.uploadError = '';
        },

        handleUploadDrop(event) {
            this.draggingUpload = false;
            this.selectUploadFiles(event.dataTransfer.files);
        },

        clearUploadQueue() {
            if (this.uploading) return;
            this.uploadFiles = [];
            this.uploadQueue = [];
            this.uploadResult = '';
            this.uploadError = '';
            const input = document.getElementById('source-upload-input');
            if (input) input.value = '';
        },

        formatBytes(bytes) {
            if (!bytes) return '0 B';
            const units = ['B', 'KB', 'MB', 'GB', 'TB'];
            let value = bytes;
            let unit = 0;
            while (value >= 1024 && unit < units.length - 1) {
                value /= 1024;
                unit += 1;
            }
            return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
        },

        uploadQueueItem(item) {
            return new Promise((resolve) => {
                const body = new FormData();
                body.append('file', item.file);
                const xhr = new XMLHttpRequest();
                xhr.open('POST', '/api/v1/upload');
                xhr.upload.onprogress = (event) => {
                    if (event.lengthComputable) {
                        item.progress = Math.round((event.loaded / event.total) * 100);
                    }
                };
                xhr.onload = () => {
                    let data = {};
                    try { data = JSON.parse(xhr.responseText || '{}'); } catch (e) {}
                    if (xhr.status >= 200 && xhr.status < 300) {
                        item.progress = 100;
                        item.status = 'queued';
                        item.jobId = data.job_id || '';
                        item.message = item.jobId ? `Queued ${item.jobId.substring(0, 8)}` : 'Queued';
                    } else {
                        item.status = 'failed';
                        item.message = data.error || `Upload failed (${xhr.status})`;
                    }
                    resolve(item);
                };
                xhr.onerror = () => {
                    item.status = 'failed';
                    item.message = 'Network error';
                    resolve(item);
                };
                xhr.send(body);
            });
        },

        async uploadSource() {
            if (!this.uploadQueue.length) return;
            this.uploading = true;
            this.uploadResult = '';
            this.uploadError = '';
            let uploaded = 0;
            let failed = 0;
            for (const item of this.uploadQueue) {
                item.status = 'uploading';
                item.message = 'Uploading...';
                item.progress = 0;
                await this.uploadQueueItem(item);
                if (item.status === 'queued') uploaded += 1;
                if (item.status === 'failed') failed += 1;
                await this.refresh();
            }
            this.uploadResult = failed === 0
                ? `${uploaded} file${uploaded === 1 ? '' : 's'} uploaded and queued.`
                : `${uploaded} uploaded, ${failed} failed.`;
            if (failed > 0) {
                this.uploadError = 'Some files failed. Check the file list below.';
            }
            this.uploading = false;
        },

        playVideo(video) {
            if (!video.playable) return;
            this.selectedVideo = video;
            this.$nextTick(() => this.attachPlayer(video.playback_url));
        },

        async deleteVideo(video) {
            if (!video || !video.job_id) return;
            if (!confirm(`Delete ${video.name || 'this video'}? This removes the source upload, HLS output, and job record.`)) return;
            try {
                const data = await apiDelete('/api/v1/media/uploads/' + video.job_id);
                if (data.error) throw new Error(data.error);
                this.videos = this.videos.filter((item) => item.job_id !== video.job_id);
                this.jobs = this.jobs.filter((job) => job.id !== video.job_id);
                await this.refresh();
            } catch (e) {
                alert(e.message || 'Delete failed');
            }
        },

        async copyShareLink(video) {
            if (!video || !video.share_url) return;
            try {
                await navigator.clipboard.writeText(video.share_url);
                this.shareMessage = 'Playback link copied';
                setTimeout(() => { this.shareMessage = ''; }, 2500);
            } catch (e) {
                this.shareMessage = video.share_url;
            }
        },

        closePlayer() {
            if (this.hlsPlayer) {
                this.hlsPlayer.destroy();
                this.hlsPlayer = null;
            }
            const player = document.getElementById('dashboard-hls-player');
            if (player) {
                player.pause();
                player.removeAttribute('src');
                player.load();
            }
            this.selectedVideo = null;
            this.qualityLevels = [];
            this.selectedQuality = '-1';
            this.playerStatus = '';
        },

        applyQuality() {
            if (!this.hlsPlayer) return;
            this.hlsPlayer.currentLevel = Number(this.selectedQuality);
        },

        levelLabel(level, index) {
            if (!level) return `Level ${index + 1}`;
            const height = level.height ? `${level.height}p` : `Level ${index + 1}`;
            const bitrate = level.bitrate ? ` · ${Math.round(level.bitrate / 1000)} kbps` : '';
            return height + bitrate;
        },

        attachPlayer(url) {
            const player = document.getElementById('dashboard-hls-player');
            if (!player || !url) return;
            if (this.hlsPlayer) {
                this.hlsPlayer.destroy();
                this.hlsPlayer = null;
            }
            this.qualityLevels = [];
            this.selectedQuality = '-1';
            this.playerStatus = 'Loading stream...';
            if (player.canPlayType('application/vnd.apple.mpegurl') && !(window.Hls && window.Hls.isSupported())) {
                player.src = url;
                this.playerStatus = 'Native adaptive HLS playback';
                player.play().catch(() => {});
                return;
            }
            if (window.Hls && window.Hls.isSupported()) {
                this.hlsPlayer = new window.Hls({ capLevelToPlayerSize: true });
                this.hlsPlayer.loadSource(url);
                this.hlsPlayer.attachMedia(player);
                this.hlsPlayer.on(window.Hls.Events.MANIFEST_PARSED, () => {
                    this.qualityLevels = this.hlsPlayer.levels || [];
                    this.selectedQuality = String(this.hlsPlayer.currentLevel ?? -1);
                    this.playerStatus = `${this.qualityLevels.length} variants available`;
                    player.play().catch(() => {});
                });
                this.hlsPlayer.on(window.Hls.Events.LEVEL_SWITCHED, (_, data) => {
                    if (this.selectedQuality === '-1') {
                        this.playerStatus = `Auto · ${this.levelLabel(this.hlsPlayer.levels[data.level], data.level)}`;
                    }
                });
                this.hlsPlayer.on(window.Hls.Events.ERROR, (_, data) => {
                    this.playerStatus = data && data.details ? data.details : 'Playback error';
                });
            } else {
                this.playerStatus = 'This browser cannot play HLS.';
            }
        },

        async refresh() {
            try {
                const [health, jobsResp, uploadsResp] = await Promise.all([
                    apiGet('/api/v1/system/health'),
                    apiGet('/api/v1/jobs?limit=10'),
                    apiGet('/api/v1/media/uploads?limit=24&thumbs=1'),
                ]);
                this.counts = health.jobs || {};
                this.jobs = jobsResp.jobs || [];
                this.videos = uploadsResp.uploads || [];
            } catch (e) {
                console.error('Dashboard refresh error:', e);
            }
        },

        startPolling() {
            this.refresh();
            setInterval(() => this.refresh(), 30000);
        }
    };
}

// Jobs page
function jobsPage() {
    return {
        jobs: [],
        total: 0,
        filter: '',
        statusClass,
        formatDuration,

        async load() {
            let url = '/api/v1/jobs?limit=100';
            if (this.filter) url += '&status=' + this.filter;
            try {
                const data = await apiGet(url);
                this.jobs = data.jobs || [];
                this.total = data.total || 0;
            } catch (e) {
                console.error('Jobs load error:', e);
            }
        },

        async retry(id) {
            await apiPost('/api/v1/jobs/' + id + '/retry');
            this.load();
        },

        async deleteJob(id) {
            if (!confirm('Delete this job?')) return;
            await apiDelete('/api/v1/jobs/' + id);
            this.load();
        }
    };
}

// System page
function systemPage() {
    return {
        info: null,
        health: null,
        analysis: null,
        analyzing: false,
        benchResult: null,
        benchmarking: false,
        benchCodec: 'libx264',
        benchRes: '1920x1080',
        scoreColor,

        async load() {
            try {
                const [info, health] = await Promise.all([
                    apiGet('/api/v1/system/info'),
                    apiGet('/api/v1/system/health'),
                ]);
                this.info = info;
                this.health = health;
            } catch (e) {
                console.error('System info load error:', e);
            }
        },

        async analyze() {
            this.analyzing = true;
            try {
                this.analysis = await apiGet('/api/v1/system/analyze');
            } catch (e) {
                console.error('Analysis error:', e);
            }
            this.analyzing = false;
        },

        async runBenchmark() {
            this.benchmarking = true;
            try {
                const [w, h] = this.benchRes.split('x').map(Number);
                this.benchResult = await apiPost('/api/v1/system/benchmark', {
                    codec: this.benchCodec,
                    width: w,
                    height: h,
                });
            } catch (e) {
                console.error('Benchmark error:', e);
            }
            this.benchmarking = false;
        }
    };
}

function settingsPage() {
    return {
        saving: false,
        uploading: false,
        message: '',
        messageType: 'success',
        uploadFile: null,
        uploadResult: '',
        ladder: [
            { name: '1080p', width: 1920, height: 1080, video_bitrate: '4100k', audio_bitrate: '128k', video_profile: 'high' },
            { name: '720p', width: 1280, height: 720, video_bitrate: '2200k', audio_bitrate: '128k', video_profile: 'high' },
            { name: '480p', width: 848, height: 480, video_bitrate: '1000k', audio_bitrate: '96k', video_profile: 'high' },
            { name: '360p', width: 640, height: 360, video_bitrate: '550k', audio_bitrate: '64k', video_profile: 'main' },
            { name: '240p', width: 426, height: 240, video_bitrate: '300k', audio_bitrate: '64k', video_profile: 'main' },
        ],
        form: {
            scanner_enabled: false,
            scanner_interval_seconds: 60,
            scanner_bucket: '',
            scanner_output_bucket: '',
            scanner_input_prefix: '',
            scanner_output_prefix: 'hls',
            scanner_output_template: 'file_base_name/hls/*',
            scanner_priority: 5,
            s3_region: 'us-central-1',
            s3_access_key: '',
            s3_secret_key: '',
            s3_secret_configured: false,
            s3_endpoint: 'https://s3.us-central-1.wasabisys.com',
            hls_video_codec: 'libx264',
            hls_video_bitrate: '2500k',
            hls_width: 1280,
            hls_height: 720,
            hls_framerate: 30,
            hls_framerate_value: '30000/1001',
            hls_gop_frames: 60,
            hls_audio_codec: 'aac',
            hls_audio_bitrate: '128k',
            hls_segment_seconds: 6,
            hls_ladder: '',
        },

        async load() {
            try {
                const data = await apiGet('/api/v1/settings');
                this.form = { ...this.form, ...data, s3_secret_key: '' };
                this.ladder = this.parseLadder(this.form.hls_ladder);
            } catch (e) {
                this.showMessage('Could not load settings.', 'error');
                console.error('Settings load error:', e);
            }
        },

        async save() {
            this.saving = true;
            this.message = '';
            try {
                const payload = { ...this.form, hls_ladder: JSON.stringify(this.cleanLadder()) };
                const data = await apiPut('/api/v1/settings', payload);
                if (data.error) {
                    this.showMessage(data.error, 'error');
                } else {
                    this.form = { ...this.form, ...data, s3_secret_key: '' };
                    this.ladder = this.parseLadder(this.form.hls_ladder);
                    this.showMessage('Settings saved. The scanner will use them on the next scan.', 'success');
                }
            } catch (e) {
                this.showMessage('Could not save settings.', 'error');
                console.error('Settings save error:', e);
            }
            this.saving = false;
        },

        showMessage(text, type) {
            this.message = text;
            this.messageType = type;
        },

        addRendition() {
            this.ladder.push({ name: '360p', width: 640, height: 360, video_bitrate: '550k', audio_bitrate: '64k', video_profile: 'main' });
        },

        removeRendition(index) {
            if (this.ladder.length <= 1) return;
            this.ladder.splice(index, 1);
        },

        parseLadder(value) {
            try {
                const parsed = JSON.parse(value || '[]');
                if (Array.isArray(parsed) && parsed.length > 0) return parsed;
            } catch (e) {
                console.error('HLS ladder parse error:', e);
            }
            return [
                { name: '1080p', width: 1920, height: 1080, video_bitrate: '4100k', audio_bitrate: '128k', video_profile: 'high' },
                { name: '720p', width: 1280, height: 720, video_bitrate: '2200k', audio_bitrate: '128k', video_profile: 'high' },
                { name: '480p', width: 848, height: 480, video_bitrate: '1000k', audio_bitrate: '96k', video_profile: 'high' },
                { name: '360p', width: 640, height: 360, video_bitrate: '550k', audio_bitrate: '64k', video_profile: 'main' },
                { name: '240p', width: 426, height: 240, video_bitrate: '300k', audio_bitrate: '64k', video_profile: 'main' },
            ];
        },

        cleanLadder() {
            return this.ladder
                .map((rendition) => ({
                    name: String(rendition.name || '').trim(),
                    width: Number(rendition.width || 0),
                    height: Number(rendition.height || 0),
                    video_bitrate: String(rendition.video_bitrate || '').trim(),
                    audio_bitrate: String(rendition.audio_bitrate || this.form.hls_audio_bitrate || '').trim(),
                    video_profile: String(rendition.video_profile || '').trim(),
                }))
                .filter((rendition) => rendition.width > 0 && rendition.height > 0 && rendition.video_bitrate);
        },

        async uploadSource() {
            if (!this.uploadFile) return;
            this.uploading = true;
            this.uploadResult = '';
            this.message = '';
            try {
                const body = new FormData();
                body.append('file', this.uploadFile);
                const resp = await fetch('/api/v1/upload', { method: 'POST', body });
                const data = await resp.json();
                if (!resp.ok || data.error) {
                    this.showMessage(data.error || 'Upload failed.', 'error');
                } else {
                    this.uploadResult = `Uploaded to s3://${data.bucket}/${data.key} and queued job ${data.job_id}`;
                }
            } catch (e) {
                this.showMessage('Upload failed.', 'error');
                console.error('Upload error:', e);
            }
            this.uploading = false;
        }
    };
}
