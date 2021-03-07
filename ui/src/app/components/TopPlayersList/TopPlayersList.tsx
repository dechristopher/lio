import React, {FC} from "react";
import classNames from "classnames";
import {List} from "@components/List/List";
import {Card} from "@components/Card/Card";


interface TempPlayerType {
	username: string;
	elo: number;
	state: "online" | "offline";
}

export const TopPlayersList: FC = () => {
	const tempData: TempPlayerType[] = [
		{username: "uptonmdlkjasndlkasndl", elo: Math.round(Math.random() * (1800)) + 700, state: "online"},
		{username: "uptonm", elo: Math.round(Math.random() * (1800)) + 700, state: "offline"},
		{username: "uptonm", elo: Math.round(Math.random() * (1800)) + 700, state: "online"},
		{username: "uptonm", elo: Math.round(Math.random() * (1800)) + 700, state: "offline"},
		{username: "uptonm", elo: Math.round(Math.random() * (1800)) + 700, state: "online"},
		{username: "uptonm", elo: Math.round(Math.random() * (1800)) + 700, state: "offline"},
	]

	return (
		<Card noPad footer={
			<div className="px-4 py-2 sm:px-2">
				<button
					type="button"
					className="w-full items-center px-2.5 py-1.5 border border-transparent text-sm font-medium rounded shadow-sm text-white bg-green-500 hover:bg-green-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-400">
					View all
				</button>
			</div>
		}>
			<div className="px-4 py-4 sm:px-6">
				<h1 className="text-2xl text-center" style={{fontWeight: 500}}>Top Players</h1>
			</div>
			<div className="px-4 py-5 sm:p-6">
				<List<TempPlayerType>
					dataSource={tempData}
					sort={(a, b) => a.elo - b.elo}
					renderItem={(record) => (
						<li className="py-4">
							<div className="flex justify-between items-center space-x-4 max-w-full">
								<div className="flex justify-start items-center space-x-4 min-w-0">
									<span className={classNames("h-3 w-3 rounded-full", {
										"bg-green-500": record.state === "online",
										"bg-transparent": record.state === "offline",
										"border-2 border-gray-300": record.state === "offline",
									})}/>
									<p className="text-md text-gray-500 overflow-hidden overflow-ellipsis">
										{record.username}
									</p>
								</div>
								<p className="flex-grow text-right text-md font-bold text-green-500 whitespace-no-wrap">
									{record.elo}
								</p>
							</div>
						</li>
					)}
				/>
			</div>
		</Card>
	)
}