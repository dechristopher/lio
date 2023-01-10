import { useRef, useEffect } from "react";

export const useAnimationFrame = (
	nextAnimationFrameHandler: (frameTime: number) => void,
	shouldAnimate: boolean,
) => {
	const frame = useRef(0);
	// keep track of when the animation is started
	const firstFrameTime = useRef(performance.now());

	const animate = () => {
		nextAnimationFrameHandler(firstFrameTime.current);
		frame.current = requestAnimationFrame(animate);
	};

	useEffect(() => {
		if (shouldAnimate) {
			firstFrameTime.current = performance.now();
			frame.current = requestAnimationFrame(animate);
		} else {
			console.log("[Animation] Canceling animation...");
			cancelAnimationFrame(frame.current);
		}

		return () => {
			console.log(
				"[Animation] Component unmounting, canceling animation...",
			);
			cancelAnimationFrame(frame.current);
		};
	}, [shouldAnimate]);
};
