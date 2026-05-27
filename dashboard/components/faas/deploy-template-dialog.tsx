'use client'

import { Loader2, Rocket } from 'lucide-react'
import type { FaasTemplate } from '@/lib/faas/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'

interface DeployTemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  templates: FaasTemplate[]
  busyTemplateId: string | null
  onDeploy: (templateId: string) => void
}

export function DeployTemplateDialog({
  open,
  onOpenChange,
  templates,
  busyTemplateId,
  onDeploy,
}: DeployTemplateDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Deploy a sandbox</DialogTitle>
          <DialogDescription>
            Pick a template to spin up an isolated Railway service. You can test endpoints and view logs once it is running.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 sm:grid-cols-2">
          {templates.map((template) => {
            const deploying = busyTemplateId === template.id
            return (
              <Card key={template.id} className="flex flex-col">
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">{template.name}</CardTitle>
                  <CardDescription className="text-xs leading-relaxed">
                    {template.description}
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-1 flex-col gap-3 pb-3">
                  <div className="flex flex-wrap gap-1.5">
                    {template.tags.map((tag) => (
                      <Badge key={tag} variant="secondary" className="text-[10px] font-normal">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                  <p className="mt-auto font-mono text-[10px] text-muted-foreground">
                    {template.testMethod} {template.testPath} · port {template.port}
                  </p>
                </CardContent>
                <CardFooter className="border-t border-border pt-3">
                  <Button
                    className="w-full"
                    size="sm"
                    disabled={busyTemplateId !== null}
                    onClick={() => onDeploy(template.id)}
                  >
                    {deploying ? (
                      <>
                        <Loader2 className="size-3.5 animate-spin" />
                        Deploying…
                      </>
                    ) : (
                      <>
                        <Rocket className="size-3.5" />
                        Deploy
                      </>
                    )}
                  </Button>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      </DialogContent>
    </Dialog>
  )
}
