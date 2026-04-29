import { palette, space } from "../../../layout/theme";

export default function ErrorBox({ text }: { text: string }) {
    return (
        <div
            style={{
                marginBottom: space[3],
                padding: `${space[3]}px ${space[4]}px`,
                border: `1px solid ${palette.danger}`,
                borderRadius: 6,
                color: palette.danger,
                fontSize: 13,
            }}
        >
            {text}
        </div>
    );
}
