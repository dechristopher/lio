import React, {FC} from "react";
import {Board} from "@components/Game/Board";
import {OctadgroundProps} from "react-octadground/octadground";
// import {Clock} from "@components/Game/Clock";

export interface GameProps {
	gameState: OctadgroundProps;
	onMove: (orig: string, dest: string) => void;
}

export const Game: FC<GameProps> = props => {
	return (
		<div className="flex flex-col" style={{width: "38vw"}}>
			{/*<Clock*/}
			{/*	elo={1400}*/}
			{/*	score={1.5}*/}
			{/*	orientation="black"*/}
			{/*	username="swanonebeau"*/}
			{/*	timeAllotted={1000 * 60 * 10}*/}
			{/*	timeRemaining={1000 * 60 * 5}*/}
			{/*/>*/}
			<Board
				height="38vw"
				width="38vw"
				{...props.gameState}
				onMove={props.onMove}
			/>
			{/*<Clock*/}
			{/*	elo={1400}*/}
			{/*	score={1.5}*/}
			{/*	orientation="white"*/}
			{/*	username="dechristopher"*/}
			{/*	timeAllotted={1000 * 60 * 10}*/}
			{/*	timeRemaining={1000 * 60 * 5}*/}
			{/*/>*/}
		</div>
	)
}