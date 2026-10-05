// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

import { createFileRoute } from "@tanstack/react-router";
import { DesktopShowcase } from "@/components/landing/desktop-showcase";
import { FeatureGrid } from "@/components/landing/feature-grid";
import { FileToResult } from "@/components/landing/file-to-result";
import { Footer } from "@/components/landing/footer";
import { Hero } from "@/components/landing/hero";
import { Nav } from "@/components/landing/nav";
import { PrivacyBand } from "@/components/landing/privacy-band";
import { Quickstart } from "@/components/landing/quickstart";
import { Reveal } from "@/components/landing/reveal";
import { ThreePlaces } from "@/components/landing/three-places";
import { strings } from "@/content/strings";
import { pageHead } from "@/lib/seo";

export const Route = createFileRoute("/")({
  head: () => pageHead({ title: strings.site.title, description: strings.site.description, path: "/" }),
  component: Home,
});

// Section order follows the approved draft.
function Home() {
  return (
    <div className="landing">
      <Nav />
      <main id="main">
        <Hero />
        <ThreePlaces />
        <FileToResult />
        <FeatureGrid />
        <DesktopShowcase />
        <PrivacyBand />
        <Quickstart />
      </main>
      <Footer />
      <Reveal />
    </div>
  );
}
