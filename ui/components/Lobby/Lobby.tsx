import { Color, VariantPool } from "@/types";
import classNames from "classnames";
import { useRouter } from "next/router";
import React, { FC, useEffect, useState } from "react";
import Button from "../Button/Button";
import ContentWrapper from "../ContentWrapper/ContentWrapper";
import styles from "./Lobby.module.scss";

interface LobbyProps {
	playerColor: Color;
	variantName: string;
	variantGroup: VariantPool;
}

const Lobby: FC<LobbyProps> = (props) => {
	const router = useRouter();
	const [hasCopied, setHasCopied] = useState(false);
	const [inviteLink, setInviteLink] = useState<string | null>(null);

	// wait for the component to mount before accessing location
	useEffect(() => {
		setInviteLink(location.href);
	}, []);

	return (
		<ContentWrapper>
			<div className="px-4 text-center pb-1">
				<div className="font-bold italic text-lg leading-none">
					Challenge to a game
				</div>

				<div className={styles.gameSettings}>
					<div className="font-semibold text-2xl leading-none">
						{`${props.variantName} ${props.variantGroup}`}
					</div>
					<div>{props.playerColor.toUpperCase()} • CASUAL</div>
				</div>

				<div
					className="mt-6"
					style={{
						maxWidth:
							"270px" /** TODO find a better way to size this */,
					}}
				>
					<div className="text-xs font-medium">
						To invite someone to play, share this URL:
					</div>
					<div className="flex">
						<input
							readOnly
							type="text"
							value={inviteLink ?? ""}
							className={styles.inviteLink}
							onClick={(event) => event.currentTarget.select()}
						/>
						<button
							disabled={!inviteLink}
							className={classNames([
								styles.copyBtn,
								{
									[styles.copied]: hasCopied,
								},
							])}
							onClick={() => {
								inviteLink &&
									navigator.clipboard.writeText(inviteLink);
								setHasCopied(true);
								setTimeout(() => setHasCopied(false), 3000);
							}}
						>
							{/* TODO replace with icons */}
							{hasCopied ? "Copied" : "Copy"}
						</button>
					</div>
					<div className="text-xs font-medium mt-3">
						Anyone that comes to this URL to choose to play with
						you.
					</div>
				</div>

				<Button
					className={styles.cancelBtn}
					onClick={() => {
						router.push("/");
					}}
				>
					x Cancel Game
				</Button>
			</div>
		</ContentWrapper>
	);
};

export default Lobby;
