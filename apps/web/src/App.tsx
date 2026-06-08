import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';

export function App() {
  return (
    <main className="min-h-screen bg-muted/30 px-6 py-10">
      <div className="mx-auto flex max-w-4xl flex-col gap-8">
        <section className="space-y-3">
          <Badge variant="secondary" className="w-fit">Local-first audio intelligence</Badge>
          <div className="space-y-2">
            <h1 className="text-4xl font-semibold tracking-tight text-balance">Soniq Web UI</h1>
            <p className="max-w-2xl text-muted-foreground">
              Upload audio, follow processing status, and review transcript and summary results from a browser.
            </p>
          </div>
        </section>

        <Card>
          <CardHeader>
            <CardTitle>Recording workflow shell</CardTitle>
            <CardDescription>
              Task 3 wires the app shell, React Query, Tailwind, shadcn/ui, and the local API proxy. Upload behavior lands in Task 4.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="title">Title</Label>
              <Input id="title" placeholder="Weekly standup" disabled />
            </div>
            <div className="space-y-2">
              <Label htmlFor="workflow-type">Workflow type</Label>
              <Select id="workflow-type" disabled defaultValue="meeting">
                <option value="meeting">Meeting</option>
                <option value="lecture">Lecture</option>
                <option value="interview">Interview</option>
                <option value="memo">Memo</option>
              </Select>
            </div>
            <div className="space-y-2 md:col-span-2">
              <Label htmlFor="audio">Audio file</Label>
              <Input id="audio" type="file" accept="audio/*" disabled />
            </div>
            <div className="flex items-center gap-3 md:col-span-2">
              <Button disabled>Upload coming in Task 4</Button>
              <span className="text-sm text-muted-foreground">API client is ready through @soniq/api-client.</span>
            </div>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
