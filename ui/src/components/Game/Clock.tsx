import React, {FC, useEffect, useState} from "react";
import ProgressBar from "@components/Progress";
import {BGColor} from "@utils/styles/colors";
import classNames from "classnames";

export type ClockOrientation = "white" | "black";

export interface ClockProps {
	elo: number;
	score: number;
	username: string;
	timeAllotted: number;
	timeRemaining: number;
	orientation: ClockOrientation;
}

export const Clock: FC<ClockProps> = (props) => {
	const [time, setTime] = useState<number>(props.timeRemaining);

	useEffect(() => {
		time > 0 && setTimeout(() => {
			setTime(time => time - 1000 );
		}, 1000);
	}, [time]);

	const formatMsTime = (time: number) => {
		const mins: number = Math.floor(time / 60000);
		const secs: number = +((time % 60000) / 1000)

		return (
			<h1 className="text-white text-2xl">
				{mins.toString().padStart(2, "0")}:{secs.toString().padStart(2, "0")}
			</h1>
		)
	}

	return (
		<div className={classNames("w-full flex flex-col bg-gray-600 p-1", {
			"flex-col-reverse": props.orientation === "white"
		})}>
			<div className={classNames("flex justify-between items-center", {
				"mt-1": props.orientation === "white",
				"mb-1": props.orientation === "black"
			})}>
				<div className="flex items-center space-x-4">
					<h1 className="text-green-200 text-2xl">{props.score}</h1>
					<h1 className="text-white text-2xl">{props.username}</h1>
				</div>

				<div className="flex justify-end items-center space-x-4">
					<h1 className="text-green-100 text-2xl">{props.elo}</h1>
					<div className="bg-green-600 flex justify-center items-center h-8 w-20">
						<h1 className="text-white text-2xl">{formatMsTime(time)}</h1>
					</div>
				</div>
			</div>
			<ProgressBar
				progress={time / props.timeAllotted}
				color={new BGColor("green")}
			/>
		</div>
	)
}