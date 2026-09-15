// Live smoke check: CPA_API_KEY=... node smoke.mjs [http://host:8317]
import { deflateSync } from 'node:zlib';

const base = process.argv[2] || 'http://127.0.0.1:8317';
let key = process.env.CPA_API_KEY;
if (!key && process.env.CPA_MANAGEMENT_KEY) {
  const response = await fetch(base + '/v0/management/config', {
    headers: { Authorization: 'Bearer ' + process.env.CPA_MANAGEMENT_KEY },
  });
  if (!response.ok) throw new Error('Cannot read CPA config: ' + response.status);
  key = (await response.json())['api-keys'][0];
}
if (!key) throw new Error('Set CPA_API_KEY or CPA_MANAGEMENT_KEY');
const headers = { Authorization: 'Bearer ' + key, 'Content-Type': 'application/json' };
const models = await (await fetch(base + '/v1/models', { headers })).json();
console.log('model:', models.data?.find(m => m.id === 'deepseek-v4.1-flash'));

// A known, losslessly encoded red PNG avoids remote image fetch ambiguity.
function crc(buffer) {
  let value = 0xffffffff;
  for (const byte of buffer) {
    value ^= byte;
    for (let i = 0; i < 8; i++) value = (value >>> 1) ^ ((value & 1) ? 0xedb88320 : 0);
  }
  return (value ^ 0xffffffff) >>> 0;
}
function chunk(type, data) {
  const tag = Buffer.from(type), size = Buffer.alloc(4), checksum = Buffer.alloc(4);
  size.writeUInt32BE(data.length);
  checksum.writeUInt32BE(crc(Buffer.concat([tag, data])));
  return Buffer.concat([size, tag, data, checksum]);
}
const ihdr = Buffer.alloc(13);
ihdr.writeUInt32BE(128, 0); ihdr.writeUInt32BE(128, 4); ihdr[8] = 8; ihdr[9] = 2;
const pixels = Buffer.alloc(128 * 385);
for (let y = 0; y < 128; y++) for (let x = 0; x < 128; x++) pixels[y * 385 + 1 + x * 3] = 255;
const png = Buffer.concat([Buffer.from([137,80,78,71,13,10,26,10]), chunk('IHDR', ihdr), chunk('IDAT', deflateSync(pixels)), chunk('IEND', Buffer.alloc(0))]);

for (const test of [
  { name: 'no-system', stream: false, content: 'Reply exactly OK.' },
  { name: 'stream-no-system', stream: true, content: 'Reply exactly OK.' },
  { name: 'vision', stream: false, content: [
    { type: 'text', text: 'What color is the image? Reply with one color word.' },
    { type: 'image_url', image_url: { url: 'data:image/png;base64,' + png.toString('base64') } },
  ] },
]) {
  const response = await fetch(base + '/v1/chat/completions', {
    method: 'POST', headers, signal: AbortSignal.timeout(60000),
    body: JSON.stringify({ model: 'deepseek-v4.1-flash', stream: test.stream,
      messages: [{ role: 'user', content: test.content }], max_tokens: 256 }),
  });
  const raw = await response.text();
  if (!response.ok) throw new Error(test.name + ': ' + response.status + ' ' + raw);
  let content = '', reasoning = '';
  if (test.stream) {
    for (const line of raw.split('\n')) {
      if (!line.startsWith('data: ') || line === 'data: [DONE]') continue;
      const event = JSON.parse(line.slice(6));
      if (event.error) throw new Error(JSON.stringify(event.error));
      content += event.choices?.[0]?.delta?.content || '';
      reasoning += event.choices?.[0]?.delta?.reasoning_content || '';
    }
  } else {
    const message = JSON.parse(raw).choices?.[0]?.message;
    content = message?.content || '';
    reasoning = message?.reasoning_content || '';
  }
  console.log(test.name, { status: response.status, content, reasoningLength: reasoning.length });
  if (!content.trim()) throw new Error(test.name + ': empty completion');
  if (test.name === 'vision' && !/red/i.test(content)) throw new Error('Vision probe failed');
}
