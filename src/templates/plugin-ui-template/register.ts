import type { PluginContext } from "@/plugin/manifest";
import { pluginRoutes } from "./routes";
import { pluginSlots } from "./slots";

export function registerPluginUI(ctx: PluginContext) {
    pluginRoutes.forEach((route) => ctx.router.addRoute(route));
    pluginSlots.forEach((slot) => {
        ctx.registerSlotComponent(slot.name, slot.component);
    });
}
