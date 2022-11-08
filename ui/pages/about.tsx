import AboutContainer from "@/components/AboutContainer/AboutContainer";
import React from "react";

export default function About() {
	return (
		<AboutContainer>
			<div>
				<div className="prose mb-3">
					Lioctad (li[bre] octad) is a free octad game server focused
					on realtime gameplay and ease of use.
				</div>
				<div className="prose mb-3">
					Octad is a chess variant that was conceived by Andrew
					DeChristopher in 2018. Rules and information about the game
					can be found below. Octad is thought to be a solved,
					deterministic game, but needs formal verification to prove
					that.
				</div>
			</div>
		</AboutContainer>
	);
}
