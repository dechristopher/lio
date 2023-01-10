import { Footer } from "@components/Footer/Footer";
import { Header } from "@components/Header/Header";
import "../styles/global.scss";
import { PieceTheme } from "@client/components/Piece/Piece";
import classNames from "classnames";

export default function RootLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<html>
			<head />
			{/* TODO retrieve from planned user preference system */}
			{/* TODO evaluate green.css theme */}
			<body className={classNames([PieceTheme.CBURNETT], "green")}>
				<div className="flex flex-col items-center pt-8">
					<Header />
					{children}
					<Footer />
				</div>
			</body>
		</html>
	);
}
