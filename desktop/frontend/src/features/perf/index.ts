// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// The performance tour runs only in the window app started with
// --perf-trace (the Plan is empty otherwise; server mode has no service).

import { Perf } from "../../lib/api";
import { serverMode } from "../../lib/mode";
import { runTour, type TourPlan } from "./tour";

if (!serverMode) {
  void Perf.Plan()
    .then(async (plan) => {
      if (!plan) return;
      try {
        await runTour(JSON.parse(plan) as TourPlan, { interactive: Perf.Interactive, record: Perf.Record });
      } catch (err) {
        await Perf.Record(`tour failed: ${String(err)}`, NaN, "");
      }
      await Perf.Done();
    })
    .catch(() => {
      // Not the window app (the service is not there).
    });
}
