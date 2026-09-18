# Mirage product video

Light-theme motion graphics product video (~92s) with voice-over.

## Render

```bash
cd marketing-video
npm install
# Voice-over already in public/vo.mp3 — regenerate with:
# edge-tts --voice en-US-JennyNeural --rate=-5% --file audio/script.txt --write-media public/vo.mp3
npm run render
```

Output: `out/mirage-product.mp4` (1920×1080, H.264 + AAC).
