import type { FaasTemplate } from './types'

export const FAAS_TEMPLATES: FaasTemplate[] = [
  {
    id: 'python-fastapi-fn',
    name: 'Python FastAPI Function',
    description: 'JSON in → JSON out API (ideal for FaaS demos and lightweight transforms).',
    tags: ['Python', 'FastAPI', 'CPU'],
    port: 8000,
    healthPath: '/health',
    testMethod: 'POST',
    testPath: '/echo',
    testBodyExample: { message: 'hello from Skyscale FaaS', n: 42 },
  },
  {
    id: 'image-processor',
    name: 'Image Processor',
    description: 'Resize & extract metadata from uploads via a containerized worker.',
    tags: ['Images', 'Pillow', 'CPU'],
    port: 8080,
    healthPath: '/health',
    testMethod: 'POST',
    testPath: '/process',
    testBodyExample: { image_url: 'https://example.com/sample.jpg', max_width: 512 },
  },
  {
    id: 'webhook-worker',
    name: 'Webhook Worker',
    description: 'Verify signatures and enqueue Stripe/GitHub-style webhook payloads.',
    tags: ['Webhooks', 'Events', 'CPU'],
    port: 3000,
    healthPath: '/health',
    testMethod: 'POST',
    testPath: '/webhook',
    testBodyExample: { type: 'checkout.session.completed', data: { object: { id: 'cs_test_123' } } },
  },
  {
    id: 'embeddings-api',
    name: 'Embeddings API',
    description: 'Batch text → vectors using a heavier dependency stack (GPU-friendly).',
    tags: ['ML', 'Embeddings', 'GPU-ready'],
    port: 8000,
    healthPath: '/health',
    testMethod: 'POST',
    testPath: '/embed',
    testBodyExample: { texts: ['skyscale faas', 'railway graphql'], model: 'demo-mini' },
  },
  {
    id: 'pdf-extractor',
    name: 'PDF Extractor',
    description: 'Parse PDFs and return structured text chunks for RAG pipelines.',
    tags: ['PDF', 'Parser', 'CPU'],
    port: 8080,
    healthPath: '/health',
    testMethod: 'POST',
    testPath: '/extract',
    testBodyExample: { url: 'https://example.com/paper.pdf', pages: [1, 2] },
  },
  {
    id: 'cron-job-runner',
    name: 'Cron Job Runner',
    description: 'Run scheduled batch jobs (reports, ETL, Slack digests) on demand.',
    tags: ['Cron', 'Batch', 'CPU'],
    port: 8080,
    healthPath: '/health',
    testMethod: 'POST',
    testPath: '/run',
    testBodyExample: { job: 'daily-report', dry_run: true },
  },
]

export function getTemplateById(id: string): FaasTemplate | undefined {
  return FAAS_TEMPLATES.find((t) => t.id === id)
}
