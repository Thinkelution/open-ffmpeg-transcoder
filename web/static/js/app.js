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

        async refresh() {
            try {
                const [health, jobsResp] = await Promise.all([
                    apiGet('/api/v1/system/health'),
                    apiGet('/api/v1/jobs?limit=10'),
                ]);
                this.counts = health.jobs || {};
                this.jobs = jobsResp.jobs || [];
            } catch (e) {
                console.error('Dashboard refresh error:', e);
            }
        },

        startPolling() {
            this.refresh();
            setInterval(() => this.refresh(), 3000);
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
        message: '',
        messageType: 'success',
        ladder: [
            { name: '1080p', width: 1920, height: 1080, video_bitrate: '5000k', audio_bitrate: '128k' },
            { name: '720p', width: 1280, height: 720, video_bitrate: '2800k', audio_bitrate: '128k' },
            { name: '480p', width: 854, height: 480, video_bitrate: '1400k', audio_bitrate: '128k' },
        ],
        form: {
            scanner_enabled: false,
            scanner_interval_seconds: 60,
            scanner_bucket: '',
            scanner_input_prefix: '',
            scanner_output_prefix: 'hls',
            scanner_output_template: 'file_base_name/hls/*',
            scanner_priority: 5,
            s3_region: 'us-east-1',
            s3_access_key: '',
            s3_secret_key: '',
            s3_secret_configured: false,
            s3_endpoint: 'https://s3.wasabisys.com',
            hls_video_codec: 'libx264',
            hls_video_bitrate: '2500k',
            hls_width: 1280,
            hls_height: 720,
            hls_framerate: 30,
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
            this.ladder.push({ name: '360p', width: 640, height: 360, video_bitrate: '800k', audio_bitrate: this.form.hls_audio_bitrate || '128k' });
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
                { name: '1080p', width: 1920, height: 1080, video_bitrate: '5000k', audio_bitrate: this.form.hls_audio_bitrate || '128k' },
                { name: '720p', width: 1280, height: 720, video_bitrate: '2800k', audio_bitrate: this.form.hls_audio_bitrate || '128k' },
                { name: '480p', width: 854, height: 480, video_bitrate: '1400k', audio_bitrate: this.form.hls_audio_bitrate || '128k' },
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
                }))
                .filter((rendition) => rendition.width > 0 && rendition.height > 0 && rendition.video_bitrate);
        }
    };
}
