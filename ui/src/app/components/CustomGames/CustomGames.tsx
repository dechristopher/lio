import React, {FC, useState} from "react";
import {Card} from "@components/Card/Card";
import {Table} from "@components/Table/Table";

export interface Game {
	playerName: string;
	elo: number;
	mode: string;
	slug: string;
}

export const CustomGames: FC = () => {
	const [gameData] = useState<Game[]>([
		{playerName: "DrNykterstein", elo: Math.round(Math.random() * (1800)) + 700, mode: "1 + 0 Rapid", slug: "gamer"},
		{playerName: "Konevlad", elo: Math.round(Math.random() * (1800)) + 700, mode: ":05 + 0 Hyper", slug: "gamer"},
		{playerName: "Zhigalko_Sergei", elo: Math.round(Math.random() * (1800)) + 700, mode: "1 + 0 Rapid", slug: "gamer"},
		{playerName: "djamir", elo: Math.round(Math.random() * (1800)) + 700, mode: ":05 + 0 Hyper", slug: "gamer"},
		{playerName: "nihalsarin2004", elo: Math.round(Math.random() * (1800)) + 700, mode: "1 + 0 Rapid", slug: "gamer"},
		{playerName: "catask", elo: Math.round(Math.random() * (1800)) + 700, mode: ":05 + 0 Hyper", slug: "gamer"},
	])

	return (
		<Card
			noPad
			header={
				<div className="px-4 py-4 sm:px-6">
					<h1 className="text-2xl text-center" style={{fontWeight: 500}}>Custom Games</h1>
				</div>
			}
			>
			<Table<Game>
				dataSource={gameData}
				columns={[
					{ title: "Player", render: (record) => record.playerName },
					{ title: "Rating", render: (record) => record.elo },
					{ title: "Mode", render: (record) => record.mode },
					// eslint-disable-next-line react/display-name
					{ title: "", render: () => <button name="join-game" className="bg-yellow-400 text-black rounded-full px-4 py-2">Join Game</button> },
				]}
			/>
		</Card>
	)
}