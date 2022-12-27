"use client";

import { usePathname } from "next/navigation";
import React from "react";
import Button from "../../components/Button/Button";
import GameSettings, { GameSettingsProps } from "./GameSettings";
import styles from "./JoinerLobby.module.scss";

export default function JoinerLobby(props: GameSettingsProps) {
	const pathName = usePathname();

	return (
		<div className="px-4 text-center pb-1">
			<div className="font-bold italic text-lg leading-none">
				Anonymous opponent
			</div>

			<GameSettings {...props} />

			<Button
				className={styles.joinBtn}
				onClick={() => {
					fetch(`/api/room${pathName}/join`, {
						method: "POST",
					});
				}}
			>
				JOIN GAME
			</Button>
		</div>
	);
}