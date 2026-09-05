import { createFileRoute } from "@tanstack/react-router";
import { Header } from "@/components/landing/Header";
import { CTA, FAQ, Features, Footer, Hero, HowItWorks } from "@/components/landing/Sections";

export const Route = createFileRoute("/")({
  head: () => ({
    meta: [
      { title: "HeapBuddy — Open-source Java/Android heap dump analyzer" },
      {
        name: "description",
        content:
          "HeapBuddy is a free, self-hostable, privacy-first .hprof analyzer. Find memory leaks, dominator trees, and wasted memory in seconds — your dumps never leave your machine.",
      },
      { property: "og:title", content: "HeapBuddy — Open-source heap dump analyzer" },
      {
        property: "og:description",
        content: "Free, self-hostable .hprof analyzer for Java and Android. Runs locally. MIT licensed.",
      },
    ],
  }),
  component: Index,
});

function Index() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Header />
      <main>
        <Hero />
        <Features />
        <HowItWorks />
        <FAQ />
        <CTA />
      </main>
      <Footer />
    </div>
  );
}
