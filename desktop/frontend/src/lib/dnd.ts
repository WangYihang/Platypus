import { PointerSensor, useSensor, useSensors } from "@dnd-kit/core";

// useDragSensors wraps the canonical dnd-kit sensor incantation that
// every drag-enabled surface in the app spelled out by hand:
//
//     const sensors = useSensors(
//         useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
//     );
//
// The `distance` activation constraint is what keeps a plain click on
// a draggable from starting a drag — without it, every click on a
// server tile or file row would register as a drag-and-drop attempt.
// 5–6 px is the default everywhere; export the constant so we don't
// have to remember which surface uses which value.
export const DEFAULT_DRAG_DISTANCE_PX = 6;

export function useDragSensors(distance = DEFAULT_DRAG_DISTANCE_PX) {
    return useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance } }),
    );
}
