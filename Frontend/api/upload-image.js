import { put } from '@vercel/blob';

export const config = {
  api: { bodyParser: false },
};

const MAX_SIZE = 5 * 1024 * 1024; // 5MB
const ALLOWED_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'];

export default async function handler(req, res) {
  if (req.method !== 'POST') {
    return res.status(405).json({ error: 'Method not allowed' });
  }

  const contentType = req.headers['content-type'] || '';
  if (!ALLOWED_TYPES.includes(contentType)) {
    return res.status(400).json({ error: 'Unsupported image type. Use PNG, JPEG, WEBP, or GIF.' });
  }

  const contentLength = parseInt(req.headers['content-length'] || '0', 10);
  if (contentLength > MAX_SIZE) {
    return res.status(413).json({ error: 'Image exceeds 5MB limit.' });
  }

  try {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const buffer = Buffer.concat(chunks);

    if (buffer.length > MAX_SIZE) {
      return res.status(413).json({ error: 'Image exceeds 5MB limit.' });
    }

    const ext = contentType.split('/')[1] || 'png';
    const filename = `market-banners/${Date.now()}-${Math.random().toString(36).slice(2)}.${ext}`;

    const blob = await put(filename, buffer, { access: 'public', contentType });

    return res.status(200).json({ url: blob.url });
  } catch (err) {
    console.error('Upload error:', err);
    return res.status(500).json({ error: 'Upload failed' });
  }
}
