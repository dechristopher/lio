import React from "react";
import AboutContainer from "@/components/AboutContainer/AboutContainer";
import HyperLink from "@/components/HyperLink/HyperLink";

export default function Misc() {
	return (
		<AboutContainer>
			<div className="about-header">
				Octad Forsyth-Edwards Notation (OFEN)
			</div>

			<div className="prose">
				Forsyth–Edwards Notation (FEN) is a standard notation for
				describing a particular board position of a chess game. The
				purpose of FEN is to provide all the necessary information to
				restart a game from a particular position.
			</div>

			<div className="prose underline">
				OFEN is a derivation of FEN to support the features of Octad.
			</div>

			<div className="prose">
				Read more about the OFEN structure
				<HyperLink
					text="here."
					link="https://github.com/dechristopher/octad/blob/master/doc/OFEN.md"
				/>
			</div>

			<div className="prose">
				Here is the OFEN for the starting position:
			</div>

			<div className="ofen-block">ppkn/4/4/NKPP w NCFncf - 0 1</div>

			<div className="prose">Here is the OFEN after the move 1. c2:</div>

			<div className="ofen-block">ppkn/4/2P1/NK1P b NCFncf - 0 1</div>
		</AboutContainer>
	);
}
