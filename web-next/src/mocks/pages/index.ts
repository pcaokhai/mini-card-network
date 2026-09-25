import { cardsHandlers } from "./cards";
import { chaosHandlers } from "./chaos";
import { labHandlers } from "./lab";
import { networkHandlers } from "./network";
import { securityHandlers } from "./security";
import { settlementHandlers } from "./settlement";

// One file per page so pages can be built in parallel without editing a shared mock file.
// These run ahead of the journey, scenario and generated handlers (MockProvider).
export const pageHandlers = [
  ...labHandlers,
  ...networkHandlers,
  ...cardsHandlers,
  ...chaosHandlers,
  ...securityHandlers,
  ...settlementHandlers,
];
