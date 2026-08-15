import { Suspense } from 'react'
import TrainingPage from '../training-page'

export default function LabPage() {
  return (
    <Suspense fallback={null}>
      <TrainingPage />
    </Suspense>
  )
}
