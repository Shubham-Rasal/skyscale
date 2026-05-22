import { Suspense } from 'react'
import TrainingPage from './training-page'

export default function Page() {
  return (
    <Suspense fallback={null}>
      <TrainingPage />
    </Suspense>
  )
}
